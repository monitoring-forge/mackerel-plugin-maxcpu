package statworker

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
)

type cpuStat struct {
	User      float64
	Nice      float64
	System    float64
	Idle      float64
	Iowait    float64
	IRQ       float64
	SoftIRQ   float64
	Steal     float64
	Guest     float64
	GuestNice float64
}

// cpuLineHeader is the prefix for the CPU line in /proc/stat
var cpuLineHeader = []byte("cpu ")

// https://github.com/prometheus/procfs/blob/c0c2a8be4d30a2e2cb95ea371a6f32a506d3e45e/proc_stat.go#L40
var userHZ float64 = 100

func GetStat() (*cpuStat, error) {
	// read /proc/stat
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get the CPU statistics from /proc/stat
	return getCPUStat(f)
}

func setCPUStat(param *float64, index int, sp [][]byte) error {
	if len(sp) > index {
		f, err := strconv.ParseFloat(string(sp[index]), 64)
		if err != nil {
			return err
		}
		*param = f / userHZ
	}
	return nil
}

// cpu  168487 7399 36999 7766545 3915 0 13480 0 0 0
// qw(cpu-user cpu-nice cpu-system cpu-idle cpu-iowait cpu-irq cpu-softirq cpu-steal cpu-guest cpu-guest-nice);
func parseCPUStatLine(line []byte) (*cpuStat, error) {
	if !bytes.HasPrefix(line, cpuLineHeader) {
		return nil, nil // continue scanning other lines
	}
	if len(line) <= len(cpuLineHeader)+1 {
		return nil, nil // continue scanning other lines
	}
	fix := 0
	if line[len(cpuLineHeader)+1] == ' ' {
		fix = 1
	}
	cs := &cpuStat{}
	sp := bytes.Fields(line[len(cpuLineHeader)+fix+1:])
	if err := setCPUStat(&cs.User, 0, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.Nice, 1, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.System, 2, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.Idle, 3, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.Iowait, 4, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.IRQ, 5, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.SoftIRQ, 6, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.Steal, 7, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.Guest, 8, sp); err != nil {
		return nil, err
	}
	if err := setCPUStat(&cs.GuestNice, 9, sp); err != nil {
		return nil, err
	}
	return cs, nil
}

func getCPUStat(f *os.File) (*cpuStat, error) {
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Bytes()
		cs, err := parseCPUStatLine(l)
		if err != nil {
			return nil, err
		}
		if cs != nil {
			return cs, nil
		}
		// continue scanning other lines
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return nil, fmt.Errorf("no cpu stats found in /proc/stat")
}
