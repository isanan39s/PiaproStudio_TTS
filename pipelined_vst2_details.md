# `pipelined.dev/audio/vst2` プラグイン利用の詳細ガイド

このドキュメントでは、Go言語でVST2プラグインを扱う `pipelined.dev/audio/vst2` パッケージ利用における、低レベルな通信プロトコル（オペコード）の詳細と、プラグインのロードから音声処理までの一般的なワークフローを解説します。

---

## 1. プラグインを読み込んでから動かすまでの一般的な流れ

GoプログラムでVSTプラグインを読み込み、音声処理を行うまでの一般的なステップを、時系列に沿ってまとめます。

### 処理フローの概要

```
[1. Open Plugin] -> [2. Prepare Host] -> [3. Init Plugin] -> [4. Configure (Optional)] -> [5. Process Audio] -> [6. Close]
```

### ステップ1: プラグインを開く (`vst2.Open`)
まず、対象のVSTプラグインファイル（`.dll` や `.vst`）をライブラリとして読み込みます。

```go
import (
    "log"
    "pipelined.dev/audio/vst2"
)

plugin, err := vst2.Open("/path/to/your/plugin.dll")
if err != nil {
    log.Fatalf("Failed to open plugin: %v", err)
}
```
内部では、プラグインの `main` 関数（または `VSTPluginMain`）が実行され、`effOpen` オペコードが呼ばれて `AEffect` 構造体が初期化されます。この時点で、`plugin.Name` や `plugin.NumParameters` などの基本情報が利用可能になります。

### ステップ2: ホストを準備する (`vst2.Host`)
次に、プラグインと通信するためのホスト側の機能（コールバック群）を準備します。

```go
import "pipelined.dev/audio/vst2"

host := vst2.Host{
    // 必要であれば、ここでコールバックを実装する
    GetTime: func() *vst2.TimeInfo {
        // 例: 現在の再生時間情報を返すロジック
        return nil // 実際には適切な値を返す
    },
    CanDo: func(s string) int32 {
        // 例: ホストの能力を返すロジック
        return 0 // 実際には適切な値を返す (vst2.HostCanDo.Yes/No)
    },
}
host.Init()
```
シンプルなエフェクト処理だけであれば `vst2.Host{}` のままでも機能しますが、プラグインがホストのテンポや再生位置に同期する必要がある場合は、`GetTime` の実装が不可欠です。

### ステップ3: プラグインを初期化する (`plugin.Init`)
準備したホストをプラグインに渡し、音声処理のための最終的な初期化を行います。

```go
if err := plugin.Init(&host); err != nil {
    log.Fatalf("Failed to initialize plugin: %v", err)
}
```
この中で、`effSetSampleRate` や `effSetBlockSize` といったオペコードが呼ばれ、プラグインは音声処理を開始できる状態になります。

### ステップ4: (任意) プラグインの事前設定
音声処理を開始する前に、プラグインのパラメータや状態を設定することができます。

#### パラメータを個別に設定
```go
// 例えば、0番目のパラメータを50%に設定
plugin.SetParameter(0, 0.5)
```

#### プリセット(チャンク)を読み込んで一括設定
```go
import "os"
chunkData, err := os.ReadFile("my_preset.fxb")
if err == nil {
    plugin.SetChunk(chunkData, false)
}
```
このステップは、特定のプリセットを再現したい場合などに使用します。

### ステップ5: 音声処理を実行する
`pipelined.dev/pipe` を使ってパイプラインを構築し、オーディオデータを流します。これが最も実用的な音声処理の方法です。

```go
import (
    "context"
    "pipelined.dev/pipe"
    "pipelined.dev/wav"
)

// inputFile, outputFile は適切に開かれた *os.File
p, err := pipe.New(
    1024, // Buffer size (例)
    pipe.Line{
        Source: &wav.Source{Reader: inputFile},
        Processors: pipe.Processors(
            // plugin.Processor() は、プラグインの音声処理機能を
            // パイプラインに組み込むためのアダプタです。
            plugin.Processor(
                // この時点でパラメータを指定することも可能
                vst2.Param("Gain", 0.75),
            ),
        ),
        Sink: &wav.Sink{Writer: outputFile},
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
パイプラインが実行されると、`Source` から読み込まれたオーディオバッファが `plugin.Processor()` に渡されます。その内部で、プラグインの `processReplacing`（または`processDoubleReplacing`）関数が呼び出され、実際の音声信号処理が行われます。

### ステップ6: リソースを解放する
すべての処理が完了したら、確保したリソースを解放します。`defer` を使って、関数の終了時に必ず実行されるようにするのが定石です。

```go
// defer plugin.Close() と defer host.Close() を main 関数の先頭で記述することが多い
defer plugin.Close()
defer host.Close()
```
`plugin.Close()` により、内部で `effClose` オペコードが呼ばれ、プラグインは自身が確保したメモリなどをクリーンアップします。

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
-   `opcode`: `effOpen` や `effSetSampleRate` など、実行したい操作のID。
-   `index`, `value`, `ptr`, `opt`: `opcode` に応じて意味が変わる汎用的な引数。これにより、一つの関数で多様な操作を実現しています。

#### 具体例: `opcode` と引数の関係

-   `effSetSampleRate` (`opcode` = 10):
    -   ホストがプラグインにサンプリングレートを設定します。
    -   `opt` (float): `44100.0` のようなサンプリングレートの値が渡されます。
    -   Goでは `plugin.Init()` の内部などで自動的に呼ばれます。

-   `effSetBlockSize` (`opcode` = 11):
    -   ホストがプラグインにブロックサイズ（一度に処理するサンプル数）を設定します。
    -   `value` (VstIntPtr): `1024` のようなブロックサイズの値が渡されます。
    -   Goでは `plugin.Init()` の内部などで自動的に呼ばれます。

-   `effGetParamName` (`opcode` = 8):
    -   ホストがパラメータの名前を要求します。
    -   `index` (VstInt32): 取得したいパラメータのインデックス番号。
    -   `ptr` (void*): プラグインがパラメータ名（文字列）を書き込むための、ホストが確保したメモリ領域へのポインタ。
    -   Goでは `plugin.Parameters()` を呼び出すと、内部でこの `opcode` が全パラメータに対して繰り返し実行され、結果が `[]vst2.Parameter` スライスにまとめられます。

-   `effSetChunk` (`opcode` = 24):
    -   ホストがプラグインにプリセット情報（チャンク）を設定します。
    -   `value` (VstIntPtr): チャンクデータのバイトサイズ。
    -   `ptr` (void*): チャンクデータ本体へのポインタ。
    -   `index` (VstInt32): これがバンク (`0`) なのかプログラム (`1`) なのかを示します。
    -   Goでは `plugin.SetChunk(data []byte, isPreset bool)` という、型安全で分かりやすいメソッドに抽象化されています。`data` が `ptr` と `value` に、`isPreset` が `index` に対応します。

### プラグインからホストへ: `audioMaster` コールバック

`audioMaster` は、`dispatcher` の逆方向の通信です。プラグインは、初期化時にホストからこのコールバック関数のポインタを受け取ります。

#### `vst2.Host` との関係

セクション7で解説した `vst2.Host` 構造体が、まさにこの `audioMaster` コールバックの実体です。

プラグインが `audioMaster(opcode, ...)` を呼び出すと、Goの `vst2` ライブラリがそれを受け取り、`vst2.Host` 構造体に登録された対応するGoの関数を呼び出します。

#### 具体例:

-   `audioMasterVersion` (`opcode` = 1):
    -   プラグインがホストのVSTサポートバージョンを問い合わせます。
    -   `vst2.Host` はデフォルトで `2400` (VST 2.4) を返します。

-   `audioMasterGetTime` (`opcode` = 7):
    -   プラグインが現在の再生位置やテンポ情報を要求します。
    -   この `opcode` を受け取ると、`vst2.Host.GetTime` フィールドに設定された `func() *vst2.TimeInfo` が呼び出されます。
    -   返された `*vst2.TimeInfo` 構造体は、ライブラリによって適切にシリアライズされ、プラグインに返されます。これが `vst2.Host` をカスタマイズする核心部分です。

-   `audioMasterCanDo` (`opcode` = 50):
    -   プラグインが「ホストは〇〇できますか？」（例: `"sendVstEvents"`）と問い合わせます。
    -   この `opcode` と引数の文字列 (`ptr`) を受け取ると、`vst2.Host.CanDo` フィールドに設定された `func(string) int32` が呼び出されます。

-   `audioMasterVendorSpecific` (`opcode` = 49):
    -   特定のホスト/プラグイン間の独自機能を実装するために使われます。
    -   `vst2.Host.VendorSpecific` フィールドに関数を設定することで、これに応答できます。

### まとめ: 抽象化の価値

VST2の低レベルAPIはC言語のポインタと型キャストに大きく依存しており、直接扱うのは非常に煩雑で危険です。

-   `effGetParamName` のために `char` の配列を確保し、ポインタを渡す。
-   `effSetChunk` のために `unsafe.Pointer` を使ってバイト列を渡す。
-   `audioMasterGetTime` の応答としてC言語互換の構造体メモリレイアウトを意識する。

`pipelined.dev/audio/vst2` のようなライブラリは、これら全ての汚い仕事を裏側でこなし、Goプログラマに対しては以下のようなクリーンなインターフェースを提供します。

-   文字列はGoの `string`
-   データブロックは `[]byte` (バイトスライス)
-   コールバックはGoの `func`

これにより、開発者はVST2仕様の低レベルな詳細を意識することなく、アプリケーションのロジックに集中できるのです。

---

## 3. オペコード詳細解説

ここでは、VST2仕様で定義されているオペコードを、できるだけ多く機能別に分類して解説します。一部の非常に稀なものや、現代の開発ではほぼ使われないものは省略していますが、GoでのVSTホスト開発において関連性の高いものを中心に、その役割、引数、そして `vst2` ライブラリとの関連性を明らかにします。

---

### **A. プラグインへのディスパッチ (`eff...` オペコード)**
ホストがプラグインの `dispatcher` 関数を呼び出してプラグインを制御します。

#### **A.1. 基本制御**
*   `effOpen` (`0`): プラグインのインスタンスが生成され、メモリ確保などの初期化処理を行うよう要求します。`vst2.Open()` の成功後、内部で呼び出されます。
*   `effClose` (`1`): プラグインのインスタンスを破棄し、確保したリソースを解放するよう要求します。Goの `plugin.Close()` に対応します。
*   `effMainsChanged` (`12`): 音声処理の一時停止/再開を伝えます。`value` が `1`でON (Resume)、`0` でOFF (Suspend) です。ディレイなどのエフェクトは、OFFの際に内部バッファをクリアすることが推奨されます。Goの `plugin.Suspend()` / `plugin.Resume()` がこれに対応します。

#### **A.2. 音声処理設定**
*   `effSetSampleRate` (`10`): サンプリングレートを設定します。引数 `opt` (float) にHz単位の値 (例: `44100.0`) が入ります。
*   `effSetBlockSize` (`11`): オーディオバッファのブロックサイズ（一度に処理するサンプル数）を設定します。引数 `value` にサンプル数 (例: `1024`) が入ります。
*   `effSetProcessPrecision` (`77`): 処理精度を設定します。`value` が `0`で32bit float、`1`で64bit floatです。`vst2` ライブラリでは通常64bitが使われます。
*   `effSetBypass` (`44`): プラグインのバイパス状態をホストが設定します。`value` が `1`でバイパスONです。`plugin.SetBypass()`が対応します。
    *(これらは主に `plugin.Init()` の過程でホストからプラグインへ通知されます。)*

#### **A.3. パラメータとプログラム**
*   `effGetParamName` (`8`), `effGetParamDisplay` (`7`), `effGetParamLabel` (`6`): `index`で指定されたパラメータの「名前」「現在の値の表示文字列 (例: "-3.0dB")」「単位 (例: "dB")」をそれぞれ要求します。プラグインは `ptr` が指すメモリ領域に文字列を書き込みます。`vst2.Parameter` 構造体の `Name`, `Display`, `Label` フィールドにマッピングされます。
*   `effGetParameterProperties` (`56`): `index`で指定されたパラメータの詳細なプロパティ（ステップ数や整数か浮動小数点数かなど）を `ptr` が指す `VstParameterProperties` 構造体に書き込ませます。
*   `effCanBeAutomated` (`26`): `index`で指定されたパラメータがオートメーション可能か問い合わせます。
*   `effString2Parameter` (`27`): `index`で指定されたパラメータに対し、`ptr`が指す文字列（例: "0.5"）を実際のパラメータ値（0.0-1.0）に変換できるか確認し、変換を実行させます。
*   `effSetProgram` (`2`), `effGetProgram` (`3`): `value` で指定/取得される現在のプログラム番号を設定・取得します。Goでは `plugin.SetProgram()` / `plugin.Program()` が対応します。
*   `effSetProgramName` (`4`), `effGetProgramName` (`5`): 現在のプログラムの名前を `ptr` を介して設定・取得します。
*   `effBeginSetProgram` (`67`), `effEndSetProgram` (`68`): `effSetProgram` の呼び出しを挟んで、プログラム変更処理の開始と終了をプラグインに伝えます。

#### **A.4. チャンク (一括データ)**
*   `effGetChunk` (`23`): 現在のプラグイン設定（全パラメータなど）を一つのバイナリデータ（チャンク）として要求します。戻り値がサイズ、`ptr` がデータ本体を指すポインタです。`index` はバンク (`0`) かプログラム (`1`) かを指定します。`plugin.Chunk()`が対応します。
*   `effSetChunk` (`24`): チャンクデータをプラグインに設定し、状態を復元します。`value` にサイズ、 `ptr` にデータ本体へのポインタが入ります。`plugin.SetChunk()`が対応します。

#### **A.5. イベント処理 (MIDI)**
*   `effProcessEvents` (`25`): ホストからプラグインへMIDIイベントなどの `VstEvents` 構造体を渡します。`ptr` にそのポインタが入ります。シンセサイザーやMIDIで制御するエフェクトに不可欠です。`vst2`ライブラリでは、ProcessorのオプションとしてMIDIイベントを渡せるようになっています。
*   `effGetNumMidiInputChannels` (`78`), `effGetNumMidiOutputChannels` (`79`): プラグインが使用するMIDIの入出力チャンネル数を取得します。

#### **A.6. 入出力、情報、能力**
*   `effGetInputProperties` (`33`), `effGetOutputProperties` (`34`): `index`で指定された入出力ピンのプロパティ（チャンネル構成など）を`ptr`が指す`VstPinProperties`構造体に取得します。
*   `effGetPlugCategory` (`35`): プラグインのカテゴリ（Effect, Synth, Analysisなど）を返します。`plugin.Category`フィールドに格納されます。
*   `effGetTailSize` (`52`): プラグインが音声停止後も音を出し続ける時間（リバーブの残響など）をサンプル数で返します。0ならテール無しです。
*   `effGetVendorString` (`47`), `effGetProductString` (`48`), `effGetVendorVersion` (`49`): プラグインの「ベンダー名」「製品名」「バージョン」を取得します。`vst2.Open` 時に内部で取得され、`plugin`構造体の各フィールドに格納されます。
*   `effCanDo` (`51`): ホストがプラグインに「〇〇できますか？」と問い合わせます。`ptr` に `"sendVstEvents"` などの文字列が入り、プラグインが可否を返します。

---

### **B. ホストへのコールバック (`audioMaster...` オペコード)**
プラグインがホストの `audioMaster` 関数を呼び出してホストの機能を要求します。`vst2.Host` 構造体の各フィールドがこれらのコールバックの実体です。

#### **B.1. 基本情報と能力**
*   `audioMasterVersion` (`1`): ホストのVSTバージョンを問い合わせます。`vst2.Host` はデフォルトで `2400` を返します。
*   `audioMasterGetVendorString` (`32`), `audioMasterGetProductString` (`33`), `audioMasterGetVendorVersion` (`34`): ホストの「ベンダー名」「製品名」「バージョン」を問い合わせます。`vst2.Host` の `GetVendorString` などのフィールドに設定した関数が呼ばれます。
*   `audioMasterCanDo` (`50`): プラグインがホストに「〇〇できますか？」（例: `openFileSelector`）と問い合わせます。`vst2.Host.CanDo` に設定した関数が `ptr` に入った文字列を引数に実行されます。

#### **B.2. 時間、イベント、状態**
*   `audioMasterGetTime` (`7`): 現在の再生時間、テンポ、拍子などの情報を要求します。`vst2.Host.GetTime` に設定した関数が呼ばれます。
*   `audioMasterProcessEvents` (`8`): プラグインが生成したMIDIイベントなどをホストに渡します。`ptr` に `VstEvents` 構造体へのポインタを渡します。`vst2.Host.ProcessEvents` が対応します。
*   `audioMasterIOChanged` (`16`): プラグインの入出力構成が変更されたことをホストに通知します。`vst2.Host.IOChanged` が対応します。
*   `audioMasterNeedIdle` (`17`), `audioMasterIdle` (`4`): プラグインがバックグラウンドで定期的に処理を実行したい場合（例: LFOの更新）、`audioMasterNeedIdle` を呼びます。ホストはこれに応じ、定期的に `effIdle` をプラグインに送ります。

#### **B.3. レイテンシーとルーティング**
*   `audioMasterGetInputLatency` (`20`), `audioMasterGetOutputLatency` (`21`): ホスト側のオーディオインターフェースなどが持つ入出力のレイテンシーを取得します。
*   `audioMasterGetSpeakerArrangement` (`37`): ホストのスピーカー構成を取得します。

#### **B.4. UIとファイル操作 (主にGUIホスト用)**
*   `audioMasterUpdateDisplay` (`60`): プラグイン側でパラメータが変更されたので、ホスト側の表示（パラメータのノブなど）も更新してほしい、と通知します。`vst2.Host.UpdateDisplay` が対応します。
*   `audioMasterOpenFileSelector` (`62`): ファイル選択ダイアログの表示を要求します。`vst2.Host.OpenFileSelector` が対応します。
*   `audioMasterBeginEdit` (`58`), `audioMasterEndEdit` (`59`): パラメータの編集を開始/終了したことをホストに伝えます。`vst2.Host.BeginEdit` / `vst2.Host.EndEdit` が対応します。
