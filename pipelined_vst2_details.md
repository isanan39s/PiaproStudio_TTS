# `pipelined.dev/audio/vst2` プラグイン利用の詳細ガイド

このドキュメントでは、Go言語でVST2プラグインを扱う `pipelined.dev/audio/vst2` パッケージ利用における、低レベルな通信プロトコル（オペコード）の詳細と、プラグインのロードから音声処理までの一般的なワークフローを解説します。

---

## 1. プラグインを読み込んでから動かすまでの一般的な流れ

GoプログラムでVSTプラグインを読み込み、音声処理を行うまでの一般的なステップを、時系列に沿ってまとめます。

### 処理フローの概要

```
[1. Open VST Library] -> [2. Prepare Host] -> [3. Create Plugin Instance & Init] -> [4. Configure (Optional)] -> [5. Process Audio] -> [6. Close]
```

### ステップ1: VSTライブラリを開き、プラグインインスタンスを作成する
まず、対象のVSTプラグインファイル（`.dll` や `.vst`）をライブラリとして読み込み、`*vst2.VST` 型のインスタンスを取得します。その後、その `*vst2.VST` インスタンスから `Plugin` メソッドを呼び出すことで、実際の `*vst2.Plugin` インスタンスを作成します。

```go
import (
    "log"
    "pipelined.dev/audio/vst2"
    "unsafe" // GoDocのPlugin.Dispatchシグネチャに含まれるため
)

// プラグインファイルをロードし、VSTライブラリへの参照を取得
vstLib, err := vst2.Open("/path/to/your/plugin.dll")
if err != nil {
    log.Fatalf("Failed to open VST library: %v", err)
}
// 関数終了時にVSTライブラリのリソースを解放することを保証
defer func() {
    if closeErr := vstLib.Close(); closeErr != nil {
        log.Printf("Error closing VST library: %v", closeErr)
    }
}()

// ステップ2で準備するホストコールバックを仮置き
// (GoDocのvst.Plugin(c HostCallbackFunc)のシグネチャに合わせるため)
dummyHostCallback := func(op vst2.HostOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) int64 {
    // 実際には、ステップ2で設定した customHost の Callback() メソッドを使う
    return 0
}

// プラグインインスタンスを作成
plugin := vstLib.Plugin(dummyHostCallback) // 初期化前のダミーコールバックで仮作成
if plugin == nil {
    log.Fatalf("Failed to create plugin instance.")
}
// 関数終了時にプラグインのリソースを解放することを保証
defer func() {
    if closeErr := plugin.Close(); closeErr != nil {
        log.Printf("Error closing plugin instance: %v", closeErr)
    }
}()
```
`vst2.Open()` の内部では、プラグインの `main` 関数（または `VSTPluginMain`）が実行され、`effOpen` オペコードが呼ばれて `AEffect` 構造体が初期化されます。この時点で、`plugin.Name()` や `plugin.NumParams()` などの基本情報が利用可能になります。

### ステップ2: ホストを準備する (`vst2.Host`)
次に、プラグインと通信するためのホスト側の機能（コールバック群）を準備します。`vst2.Host` 構造体は、プラグインがホストに情報を要求したり、何かをお願いしたりする際に呼ばれる関数を保持します。

```go
import (
    "pipelined.dev/audio/vst2"
    "pipelined.dev/signal" // signal.Frequencyのために必要
    "time" // GetTimeInfoの例で使用
)

// customHost の GetTimeInfo にて使用する TimeInfo を作成する例
// (GoDocの HostGetTimeInfoFunc のシグネチャに合わせたダミー)
var currentTimeInfo = vst2.TimeInfo{
    Tempo:          120.0,
    TimeSigNumerator: 4,
    TimeSigDenominator: 4,
    Transporting:   1, // 再生中
    SampleRate: 44100.0, // ダミー
}

host := vst2.Host{
    // ホストのサンプリングレートをプラグインに伝えるコールバック
    GetSampleRate: func() signal.Frequency { return 44100 },
    // ホストのバッファサイズをプラグインに伝えるコールバック
    GetBufferSize: func() int { return 1024 },
    // ホストの現在の処理レベルをプラグインに伝えるコールバック
    GetProcessLevel: func() vst2.ProcessLevel { return vst2.ProcessLevelRealtime },
    // ホストの現在の時間情報をプラグインに伝えるコールバック
    GetTimeInfo: func(flags vst2.TimeInfoFlag) *vst2.TimeInfo {
        // 実際のアプリケーションでは、ここに現在の再生時間やテンポなどを動的に計算して設定
        return &currentTimeInfo
    },
    // GoDocで明示されていないが、CanDoやその他のコールバックも必要に応じて実装する
    // VST SDKの HostCallbackFunc が内部でこれらのオペコードを処理する
}

// GoDocのvst2.Hostには直接Init()メソッドは存在しません。
// 概念的なホストの「準備」として、フィールドに関数を設定する形になります。
// customHost.Init() // GoDocに存在しないためコメントアウト
```
`vst2.Host` の各フィールドに関数を設定することで、プラグインの要求に応答できるようになります。特に `GetTimeInfo` の実装は、テンポ同期が必要なプラグインにとって重要です。

### ステップ3: プラグインの初期化とホストコールバックの設定
準備したホスト情報をプラグインに渡し、音声処理のための最終的な初期化を行います。

```go
// vstLib.Plugin(c HostCallbackFunc) の引数として、準備したホストの Callback() メソッドを渡す
// これにより、plugin インスタンスがホストと通信できるようになる
plugin = vstLib.Plugin(host.Callback())
if plugin == nil {
    log.Fatalf("Failed to create plugin instance with real host callback.")
}

// GoDocの Plugin 型には Init() メソッドは明示されていませんが、
// vstLib.Processor().Allocator() の ProcessorInitFunc や
// plugin.SetSampleRate(), plugin.SetBufferSize(), plugin.Resume()などを
// 適切に呼び出すことで、プラグインは初期化されます。
// ここでは、概念的なステップとして記述を維持しますが、直接の Init() 呼び出しは不要です。
// (旧来の Init() は Resume() などと等価と見なせるため)
// plugin.Init() // GoDocに存在しないためコメントアウト
```
このステップで、`effSetSampleRate` や `effSetBlockSize` といったオペコードがプラグインに通知され、プラグインは音声処理を開始できる状態になります。

### ステップ4: (任意) プラグインの事前設定
音声処理を開始する前に、プラグインのパラメータや状態を設定することができます。

#### パラメータを個別に設定
```go
// 例えば、0番目のパラメータを50%に設定
if plugin.NumParams() > 0 { // GoDoc: plugin.NumParams() でパラメータ数を取得
    paramIndex := 0
    newValue := float32(0.5) // GoDoc: SetParamValue は float32 を取る
    plugin.SetParamValue(paramIndex, newValue) // GoDoc: plugin.SetParamValue() メソッド
    
    // 設定後の値と表示名を確認 (GoDoc: ParamValue, ParamValueName メソッド)
    fmt.Printf("Parameter %d ('%s') set to %.2f (Display: %s)\n",
        paramIndex,
        plugin.ParamName(paramIndex),       // GoDoc: plugin.ParamName()
        plugin.ParamValue(paramIndex),      // GoDoc: plugin.ParamValue()
        plugin.ParamValueName(paramIndex),  // GoDoc: plugin.ParamValueName()
    )
}
```

#### プリセット(チャンク)を読み込んで一括設定
```go
import "os"
chunkData, err := os.ReadFile("my_preset.fxb")
if err == nil {
    // GoDoc: plugin.SetProgramData() や plugin.SetBankData() を使用
    plugin.SetProgramData(chunkData) // プログラムチャンクとして設定する例
    fmt.Println("Loaded preset from my_preset.fxb")
}
```
このステップは、特定のプリセットを再現したい場合などに使用します。GoDocの `SetProgramData` や `SetBankData` メソッドが対応します。

### ステップ5: 音声処理を実行する
`pipelined.dev/pipe` を使ってパイプラインを構築し、オーディオデータを流します。これが最も実用的な音声処理の方法です。

```go
import (
    "context"
    "pipelined.dev/pipe"
    "pipelined.dev/wav"
)
import "pipelined.dev/audio/vst2" // vstLib.Processor() で必要

// inputFile, outputFile は適切に開かれた *os.File
// (この例では外部で開かれているものとする)
p, err := pipe.New(
    1024, // Buffer size (例)
    pipe.Line{
        Source: &wav.Source{Reader: inputFile}, // input.wav の Reader
        Processors: pipe.Processors(
            // VSTライブラリからProcessorを作成し、パイプラインに組み込む
            // GoDoc: vstLib.Processor(h Host, progressFn ProgressProcessedFunc) *Processor
            // Host を渡し、必要に応じて ProgressProcessedFunc も設定
            vstLib.Processor(host, nil).Allocator(func(p *vst2.Plugin){
                // ProcessorInitFunc (GoDoc参照) でプラグインの最終設定を行える
                // 例えば、プラグインが64bit処理をサポートしているか確認し、
                // 必要なら SetSampleRate や SetBufferSize を呼ぶ
                if p.CanProcessFloat64() { // GoDoc: Plugin.CanProcessFloat64()
                    fmt.Println("Plugin supports float64 processing.")
                    p.SetProcessPrecision(vst2.ProcessDouble) // GoDoc: Plugin.SetProcessPrecision()
                } else {
                    p.SetProcessPrecision(vst2.ProcessFloat)
                }
                p.SetSampleRate(signal.Frequency(44100)) // GoDoc: Plugin.SetSampleRate()
                p.SetBufferSize(1024)                    // GoDoc: Plugin.SetBufferSize()
                p.Resume()                               // GoDoc: Plugin.Resume() 処理開始
            }),
        ),
        Sink: &wav.Sink{Writer: outputFile}, // output.wav の Writer
    },
)
if err != nil {
    log.Fatalf("Failed to build pipe: %v", err)
}
// パイプラインを実行
err = pipe.Wait(p.Start(context.Background()))
if err != nil {
    log.Fatalf("Failed to execute pipe: %v", err)
}
```
パイプラインが実行されると、`Source` から読み込まれたオーディオバッファが `vstLib.Processor(...)` に渡されます。その内部で、プラグインの `ProcessDouble` または `ProcessFloat` 関数（GoDoc参照）が呼び出され、実際の音声信号処理が行われます。

### ステップ6: リソースを解放する
すべての処理が完了したら、確保したリソースを解放します。`defer` を使って、関数の終了時に必ず実行されるようにするのが定石です。

```go
// defer vstLib.Close() と defer plugin.Close() は main 関数の先頭で記述することが多い
// host.Close() は GoDoc の Host 型に直接 Close メソッドがないため、ここでは割愛
```
`vstLib.Close()` は、GoDocの `VST` 型に明示されている `Close()` メソッドに対応します。`plugin.Close()` も同様です。

---

## 2. VST2通信の核心: DispatcherとAudioMaster

ご提示いただいた仕様書は、VST2のホストとプラグイン間の通信の心臓部である「ディスパッチャモデル」を定義しています。ここでは、この低レベルな仕組みと、Goの `vst2` パッケージがそれをどう扱っているかを解説します。

### 双方向の通信モデル

VST2の通信は、C言語ベースのシンプルな関数呼び出しで行われます。

1.  **ホスト → プラグイン**: ホストがプラグインに何かを命令・設定する場合、プラグインが公開している `dispatcher` 関数を呼び出します。
2.  **プラグイン → ホスト**: プラグインがホストに情報を要求したり、何かをお願いしたりする場合、ホストから提供された `audioMaster` コールバック関数を呼び出します。

```
+-----------+                              +-------------+
|           | -- dispatcher(opcode, ...) -> |             |
|   Host    |                              |   Plugin    |
| (Go App)  | <- audioMaster(opcode, ...) -- | (.dll/.vst) |
|           |                              |             |
+-----------+                              +-------------+
```

このモデルの鍵は **`opcode`（オペレーションコード）** です。これは実行したい操作の種類を識別する整数値で、`dispatcher` と `audioMaster` の両方で使われます。

### ホストからプラグインへ: `dispatcher` の仕組み

仕様書にある `dispatcher` 関数のC++シグネチャは以下の通りです。

```cpp
VstIntPtr dispatcher(AEffect* e, VstInt32 opcode, VstInt32 index, VstIntPtr value, void* ptr, float opt);
```

-   `e`: プラグインのインスタンス自身へのポインタ。
-   `opcode`: `effOpen` や `effSetSampleRate` など、実行したい操作のID。GoDocでは `vst2.PluginOpcode` として定義されています。
-   `index`, `value`, `ptr`, `opt`: `opcode` に応じて意味が変わる汎用的な引数。これにより、一つの関数で多様な操作を実現しています。
    GoDocの `func (p *Plugin) Dispatch(opcode PluginOpcode, index int32, value int64, ptr unsafe.Pointer, opt float32) uintptr` メソッドがこれに直接対応します。

#### 具体例: `opcode` と引数の関係

-   `effSetSampleRate` (`opcode` = 10):
    -   ホストがプラグインにサンプリングレートを設定します。
    -   `opt` (float): `44100.0` のようなサンプリングレートの値が渡されます。
    -   GoDocの `func (p *Plugin) SetSampleRate(sampleRate signal.Frequency)` メソッドがこのオペコードをラップしています。

-   `effSetBlockSize` (`opcode` = 11):
    -   ホストがプラグインにブロックサイズ（一度に処理するサンプル数）を設定します。
    -   `value` (VstIntPtr): `1024` のようなブロックサイズの値が渡されます。
    -   GoDocの `func (p *Plugin) SetBufferSize(bufferSize int)` メソッドがこのオペコードをラップしています。

-   `effGetParamName` (`opcode` = 8):
    -   ホストがパラメータの名前を要求します。
    -   `index` (VstInt32): 取得したいパラメータのインデックス番号。
    -   `ptr` (void*): プラグインがパラメータ名（文字列）を書き込むための、ホストが確保したメモリ領域へのポインタ。
    -   GoDocの `func (p *Plugin) ParamName(index int) string` メソッドがこのオペコードをラップし、Goの文字列として返します。

-   `effSetChunk` (`opcode` = 24):
    -   ホストがプラグインにプリセット情報（チャンク）を設定します。
    -   `value` (VstIntPtr): チャンクデータのバイトサイズ。
    -   `ptr` (void*): チャンクデータ本体へのポインタ。
    -   `index` (VstInt32): これがバンク (`0`) なのかプログラム (`1`) なのかを示します。
    -   GoDocの `func (p *Plugin) SetProgramData(data []byte)` や `func (p *Plugin) SetBankData(data []byte)` メソッドがこのオペコードをラップし、`[]byte` スライスを受け取ります。

### プラグインからホストへ: `audioMaster` コールバック

`audioMaster` は、`dispatcher` の逆方向の通信です。プラグインは、初期化時にホストからこのコールバック関数のポインタを受け取ります。

#### `vst2.Host` との関係

セクション7で解説した `vst2.Host` 構造体が、まさにこの `audioMaster` コールバックの実体です。

プラグインが `audioMaster(opcode, ...)` を呼び出すと、Goの `vst2` ライブラリがそれを受け取り、`vst2.Host` 構造体に登録された対応するGoの関数を呼び出します。GoDocの `Host` 型に定義されている `GetSampleRate`, `GetBufferSize`, `GetProcessLevel`, `GetTimeInfo` などが、この `audioMaster` コールバックの特定の `opcode` に応答するためのフィールドです。

#### 具体例:

-   `audioMasterVersion` (`opcode` = 1):
    -   プラグインがホストのVSTサポートバージョンを問い合わせます。GoDocの `vst2.HostVersion` (`HostOpcode`) に対応します。

-   `audioMasterGetTime` (`opcode` = 7):
    -   プラグインが現在の再生位置やテンポ情報を要求します。GoDocの `vst2.HostGetTime` (`HostOpcode`) に対応。
    -   この `opcode` を受け取ると、`vst2.Host.GetTimeInfo` フィールドに設定された `HostGetTimeInfoFunc` が呼び出されます。
    -   返された `*vst2.TimeInfo` 構造体は、ライブラリによって適切にシリアライズされ、プラグインに返されます。これが `vst2.Host` をカスタマイズする核心部分です。

-   `audioMasterCanDo` (`opcode` = 50):
    -   プラグインが「ホストは〇〇できますか？」（例: `