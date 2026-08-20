package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	connect "github.com/bufbuild/connect-go"
	"github.com/monitoring-forge/flagrun"
	"github.com/monitoring-forge/mackerel-plugin-maxcpu/internal/statworker"
	maxcpuconnect "github.com/monitoring-forge/mackerel-plugin-maxcpu/maxcpu/maxcpuconnect"
	"google.golang.org/protobuf/types/known/emptypb"
)

var version string

type Opt struct {
	Socket   string `short:"s" long:"socket" required:"true" description:"Socket file used calcurating daemon" `
	AsDaemon bool   `long:"as-daemon" description:"run as daemon"`
	Version  bool   `short:"v" long:"version" description:"Show version"`
	client   maxcpuconnect.MaxCPUClient
}

func runBinaryCheck(socket string, current time.Time) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		modified, err := selfModified()
		if err != nil {
			continue
		}
		if modified != current {
			cmd := exec.Command(os.Args[0], "--as-daemon", "--socket", socket)
			errCmd := cmd.Start()
			if errCmd != nil {
				fmt.Fprintf(os.Stderr, "%v\n", errCmd)
			} else {
				time.Sleep(10 * time.Second)
				// sockファイルを消さないようsigkillで止める
				errKill := syscall.Kill(syscall.Getpid(), syscall.SIGKILL)
				if errKill != nil {
					fmt.Fprintf(os.Stderr, "%v\n", errKill)
				}
			}
		}
	}
}

func selfModified() (time.Time, error) {

	fs, err := os.Stat(os.Args[0])
	if err != nil {
		return time.Now(), err
	}
	return fs.ModTime(), nil
}

func (opt *Opt) execBackground() error {
	// check proc before exec
	_, err := statworker.GetStat()
	if err != nil {
		return err
	}

	cmd := exec.Command(os.Args[0], "--as-daemon", "--socket", opt.Socket)
	err = cmd.Start()
	if err != nil {
		return err
	}
	return nil
}

var maxIdleTime int64 = 600

func runIdleCheck(w *statworker.Worker) {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		if w.IdleTime() > maxIdleTime {
			err := syscall.Kill(syscall.Getpid(), syscall.SIGKILL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
		}
	}
}

func (opt *Opt) runBackground() error {

	modified, err := selfModified()
	if err != nil {
		return err
	}

	worker := statworker.New()

	go func() { worker.Run() }()
	go func() { runIdleCheck(worker) }()
	go func() { runBinaryCheck(opt.Socket, modified) }()

	time.Sleep(1 * time.Second)

	mux := http.NewServeMux()
	path, handler := maxcpuconnect.NewMaxCPUHandler(worker)
	mux.Handle(path, handler)

	idleConnsClosed := make(chan struct{})
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM, os.Interrupt)
		<-sigChan
		close(idleConnsClosed)
	}()

	err = os.Remove(opt.Socket)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	unixListener, err := net.Listen("unix", opt.Socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(opt.Socket, 0600); err != nil {
		return err
	}
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(unixListener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
	}()
	<-idleConnsClosed
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	return nil
}

func (opt *Opt) checkDaemonAlive() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req := connect.NewRequest(&emptypb.Empty{})
	_, err := opt.client.Hello(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check daemon alive failed: %v\n", err)
		return false
	}
	return true
}

func (opt *Opt) getStats() error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	res, err := opt.client.GetStats(ctx, connect.NewRequest(&emptypb.Empty{}))
	if err != nil {
		return err
	}
	for _, m := range res.Msg.Metrics {
		fmt.Printf(
			"maxcpu.us_sy_wa_si_st_usage.%s\t%f\t%d\n",
			m.Key,
			m.Metric,
			m.Epoch,
		)
	}
	return nil
}

func makeClient(socket string) (maxcpuconnect.MaxCPUClient, error) {
	uid := os.Geteuid()
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// check owner of socket file
				fi, err := os.Stat(socket)
				if err != nil {
					return nil, err
				}
				stat, ok := fi.Sys().(*syscall.Stat_t)
				if !ok {
					return nil, fmt.Errorf("failed to get socket file stat")
				}
				if int(stat.Uid) != uid {
					return nil, fmt.Errorf("socket file owner is not current user")
				}
				ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
				defer cancel()
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
	baseURL := "http://unix"
	c := maxcpuconnect.NewMaxCPUClient(httpClient, baseURL)
	return c, nil
}

func (opt *Opt) Run(_ []string) (error, int) {
	if opt.AsDaemon {
		err := opt.runBackground()
		if err != nil {
			return err, flagrun.CRITICAL
		}
		return nil, flagrun.OK
	}

	client, err := makeClient(opt.Socket)
	if err != nil {
		return err, flagrun.CRITICAL
	}
	opt.client = client

	if !opt.checkDaemonAlive() {
		// exec daemon
		fmt.Fprintf(os.Stderr, "start background process\n")
		err := opt.execBackground()
		if err != nil {
			return err, flagrun.CRITICAL
		}
		return nil, flagrun.OK
	}

	err = opt.getStats()
	if err != nil {
		return err, flagrun.CRITICAL
	}
	return nil, flagrun.OK

}

func main() {
	os.Exit(flagrun.Go(&Opt{}, flagrun.Version(version)))
}
