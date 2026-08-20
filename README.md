# mackerel-plugin-maxcpu

> Linux の CPU 使用率を最大値・最小値・平均値などで監視できる Mackerel プラグイン

[![Go Report Card](https://goreportcard.com/badge/github.com/monitoring-forge/mackerel-plugin-maxcpu)](https://goreportcard.com/report/github.com/monitoring-forge/mackerel-plugin-maxcpu)

## 概要

`mackerel-plugin-maxcpu` は、指定した期間内の CPU 使用率を **最大値(Max)・最小値(Min)・平均値(Avg)・75パーセンタイル・90パーセンタイル** として集計し、Mackerel に送信するためのプラグインです。

単なる瞬間値ではなく、一定期間の統計情報を取得することで、CPU のピーク負荷や傾向を把握することに優れています。

**対応OS: Linux のみ**

## インストール

### Mackerel コマンドでインストール

最も簡単な方法は、Mackerel のプラグインインストールコマンドを使用することです。

```bash
mkr plugin install monitoring-forge/mackerel-plugin-maxcpu
```

### バイナリをダウンロード

[Releaseページ](https://github.com/monitoring-forge/mackerel-plugin-maxcpu/releases) から自分の環境に適したバイナリをダウンロードして、`/usr/local/bin` などに配置します。

```bash
# Linux amd64 の例
curl -LO https://github.com/monitoring-forge/mackerel-plugin-maxcpu/releases/download/v0.0.19/mackerel-plugin-maxcpu-linux-amd64
sudo install mackerel-plugin-maxcpu-linux-amd64 /usr/local/bin/mackerel-plugin-maxcpu
```

## 使い方

### 基本コマンド

```bash
Usage:
  mackerel-plugin-maxcpu [OPTIONS]

Application Options:
  -s, --socket=    ダーモンが使用するソケットファイルのパス
      --as-daemon  起動時に背景でダーモンとして動作する
  -v, --version    バージョン情報を表示

Help Options:
  -h, --help       このヘルプメッセージを表示
```

### 初回実行と以降

初回実行時に自動でバックグラウンドデーモンが起動します。2回目以降の実行では、既に動作しているデーモンに接続して統計情報を取得します。

```bash
# 初回実行（デーモンが自動起動します）
$ sudo ./mackerel-plugin-maxcpu --socket /var/run/maxcpu.sock
check daemon alive failed: rpc error: ...
start background process

# 2回目以降（デーモンに接続してメトリクスを取得）
$ sudo ./mackerel-plugin-maxcpu --socket /var/run/maxcpu.sock
maxcpu.us_sy_wa_si_st_usage.max 0.251256        1604022058
maxcpu.us_sy_wa_si_st_usage.min 0.250627        1604022058
maxcpu.us_sy_wa_si_st_usage.avg 0.250941        1604022058
maxcpu.us_sy_wa_si_st_usage.90pt        0.251256        1604022058
maxcpu.us_sy_wa_si_st_usage.75pt        0.251256        1604022058
```

## 設定例

`mackerel-plugin-maxcpu` は Mackerel のエージェント設定ファイル (`/etc/mackerel-agent/mackerel-agent.conf`) に以下のように設定します。

```ini
[plugin.metrics.maxcpu]
Command = ["mackerel-plugin-maxcpu", "-s", "/var/run/maxcpu.sock"]
```

設定後に Mackerel エージェントを再起動します。

```bash
sudo systemctl restart mackerel-agent
```

## 出力されるメトリクス

### メトリクスの一覧

| メトリクス名 | 説明 | 単位 |
|---|---|---|
| `maxcpu.<category>.usage.max` | 期間内の最大 CPU 使用率 | % |
| `maxcpu.<category>.usage.min` | 期間内の最小 CPU 使用率 | % |
| `maxcpu.<category>.usage.avg` | 期間内の平均 CPU 使用率 | % |
| `maxcpu.<category>.usage.90pt` | 90パーセンタイルの CPU 使用率 | % |
| `maxcpu.<category>.usage.75pt` | 75パーセンタイルの CPU 使用率 | % |

### カテゴリ（プレフィックス）

| プレフィックス | 内容 |
|---|---|
| `us` | ユーザープロセスの CPU 使用率（user time） |
| `sy` | カーネルプロセスの CPU 使用率（system time） |
| `wa` | I/O ウェイト（I/O を待機中） |
| `si` | ソフトウェア割り込み処理（softirq） |
| `st` | 仮想環境での他VMへの取り忘れ（steal time） |

これらのカテゴリを組み合わせた名前でも使用できます。

- `us_sy` — ユーザー + システム
- `us_sy_wa` — ユーザー + システム + I/Oウェイト
- `us_sy_wa_si_st` — 全てのカテゴリを合計

### メトリクスの意味

- **max（最大値）**: 計測期間の中で最も高い CPU 使用率。スパイク負荷の検出に有用
- **min（最小値）**: 計測期間の中で最も低い CPU 使用率。アイドル状態の確認に有用
- **avg（平均値）**: 計測期間の平均 CPU 使用率。全体の負荷傾向を把握 on 有用
- **90pt（90パーセンタイル）**: 90%の期間がこの値以下。ピーク負荷の傾向把握に有用
- **75pt（75パーセンタイル）**: 75%の期間がこの値以下。通常のピークを把握 on 有用

## 動作の仕組み

### アーキテクチャ

```
┌─────────────────────┐          ┌──────────────────┐
│  mackerel-agent     │          │  mackerel-plugin  │
│  (每隔1分実行)       │─────────▶│  (デーモンモード)  │
└─────────────────────┘ Unix RPC └──────────────────┘
                                        │
                                        ▼
                                  ┌──────────────┐
                                  │  /proc/stat   │
                                  │  (CPU統計情報) │
                                  └──────────────┘
```

1. **デーモン起動**: 初回実行時に `--as-daemon` フラグ付きでバックグラウンドプロセスが起動します
2. **CPU統計の収集**: デーモンは Linux の `/proc/stat` から CPU 統計情報を定期的に取得します
3. **履歴の保持**: 最大361件のCPU使用率履歴を保持します（約6分間のデータ）
4. **RPC通信**: Mackerel プラグイン実行ごとに、Unixソケット経由でデーモンに接続し集計結果を取得します
5. **自動アイドル停止**: デーモンが600秒（10分）間アイドル状態になると自動的に終了します

### 内部動作の詳細

- `/proc/stat` から `cpu` ラインの値を読み取り、ユーザー・システム・アイドルなどの時間を秒単位で計算
- 前回の値との差分から、期間内の使用率を算出
- 円形バッファ（リングバッファ）に履歴を保持し、最新361件のデータを使用して統計を計算
- プログラムのバイナリが更新された場合は自動でデーモンを再起動



## License

[MIT License](LICENSE)

## Links

- [Blog Entry](https://kazeburo.hatenablog.com/entry/2020/11/09/134207) (日本語)
- [Mackerel](https://mackerel.io/)

