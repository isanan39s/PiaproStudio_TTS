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
