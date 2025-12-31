
cgoはGoプログラムがCのコードを直接呼び出すための仕組みです。C++のコードはそのままでは呼べないため、C++の機能をCの関数でラップ（包み込む）する必要があります。

# 全体的な流れ
   1. C++側: Win32 APIを使ってウィンドウを作成・管理するコードをC++で記述します。
   2. Cラッパー: Goから呼び出したいC++の機能を、extern "C"を付けてC言語互換の関数として公開します。
   3. コンパイル: C++のコードを共有ライブラリ（Windowsの場合は.dll）または静的ライブラリ（.a）としてコンパイルします。
   4. Go側: Goのソースコード内でcgoのディレクティブ（コメント形式の命令）を使い、コンパイルしたライブラリをリンクして、Cの関数を呼び
      出します。

  ---

## 基本的な仕組み

   1. C++側:
		* Goから呼び出したい関数をextern "C"でラップします。これにより、C++の関数がC言語のリンケージ規約でエクスポートされ、cgoから見えるようになります。
	   * C++のコードをコンパイルして、静的ライブラリ (.a) または共有ライブラリ (.dll) を作成します。

   2. Go側:
	   * import "C"という特別なインポート文を使います。
		* #cgoディレクティブ（コメントとして記述）を使って、C/C++コンパイラへのフラグ（インクルードパス、ライブラリパスなど）を指定します。
		* C.関数名()のようにして、C++側でエクスポートした関数を呼び出します。
		
		
### 簡単な実装例

以下に、C++でウィンドウを作成して表示し、その処理をGoから呼び出す簡単な例を示します。
1. C++側のコードを作成
  *  ウィンドウの生成とメッセージループを処理するC++コードです。Goから呼び出すためのShowWindowFromGoという関数をextern "C"で公開しています。
	
```c++:window.cpp
// .cpp
#include <windows.h>

// ウィンドウプロシージャ
LRESULT CALLBACK WndProc(HWND hwnd, UINT msg, WPARAM wParam, LPARAM lParam) {
    switch (msg) {
        case WM_DESTROY:
            PostQuitMessage(0);
            break;
        default:
            return DefWindowProc(hwnd, msg, wParam, lParam);
    }
    return 0;
}
// extern "C" を使って、C言語形式で関数をエクスポートする
#ifdef __cplusplus
extern "C" {
#endif
__declspec(dllexport) void ShowWindowFromGo() {
    const char CLASS_NAME[] = "SampleWindowClass";
    WNDCLASS wc = {};
    wc.lpfnWndProc   = WndProc;
    wc.hInstance     = GetModuleHandle(NULL);
    wc.lpszClassName = CLASS_NAME;
    RegisterClass(&wc);
    HWND hwnd = CreateWindowEx(
        0,
        CLASS_NAME,
        "Window from C++",
        WS_OVERLAPPEDWINDOW,
        CW_USEDEFAULT, CW_USEDEFAULT, 500, 300,
        NULL, NULL, GetModuleHandle(NULL), NULL
    );
    if (hwnd == NULL) {
        return;
    }
    ShowWindow(hwnd, SW_SHOW);
    UpdateWindow(hwnd);
    // メッセージループ
    MSG msg = {};
    while (GetMessage(&msg, NULL, 0, 0)) {
        TranslateMessage(&msg);
        DispatchMessage(&msg);
    }
}
#ifdef __cplusplus
}
#endif
```

  * これらのC++ファイルを静的ライブラリ（libmsgbox.a）としてコンパイルします。これにはg++（MinGWなど）が必要です。

  * 上記のwindow.cppを共有ライブラリ（DLL）としてコンパイルします。これにはMinGW-w64などのGCCツールチェインが必要です。

	次のコマンドで window.dll を生成します。
		`g++ -shared -o window.dll window.cpp -luser32 -lgdi32`

2. Go側のコード (main.go)
  Goのメインプログラムです。cgoを使ってwindow.dllを読み込み、ShowWindowFromGo関数を呼び出します。

```go:main.go
 // main.go
 package main
 
 /*
 #cgo LDFLAGS: -L. -lwindow
 void ShowWindowFromGo();
 */
 import "C"
 import "fmt"
 
 func main() {
     fmt.Println("Go program started. Calling C++ to create a window...")
 
     // C++で実装された関数を呼び出す
     // この関数はウィンドウが閉じるまでブロックします
     C.ShowWindowFromGo()
 
     fmt.Println("Window was closed. Go program finished.")
 }
```
  cgoディレクティブの説明:

   * #cgo LDFLAGS: -L. -lwindow: リンカフラグです。
       * -L. : カレントディレクトリ（.）をライブラリの検索パスに追加します。
       * -lwindow : libwindow.dll（Windowsではlibプレフィックスが省略されてwindow.dll）をリンクします。
   * void ShowWindowFromGo();: GoにCの関数シグネチャを教えます。

 go buildコマンドでビルドし、生成されたmain.exeを実行します。

 Goプログラムをビルド
`go build`

 実行
`./main.exe`

  実行すると、コンソールに"Go program
  started..."と表示された後、C++で記述されたウィンドウが表示されます。そのウィンドウを閉じると、メッセージループが終了し、Goプ
  ログラムも"Window was closed..."と表示して正常に終了します。

  注意点

   * ビルド環境: cgoを使用するには、gccなどのC/C++コンパイラがシステムにインストールされている必要があります。
   * データ型: GoとCの間で文字列や複雑な構造体をやり取りする場合、C.CStringやポインタの変換など、型変換に注意が必要です。
   * ブロッキング:
     この例のShowWindowFromGo関数は、内部でメッセージループを実行するため、ウィンドウが閉じるまでGo側へ制御が戻りません。もしG
     oの処理と並行してウィンドウを操作したい場合は、メッセージループを別のスレッド（Goのgoroutineなど）で実行するなどの工夫が
     必要になります。
