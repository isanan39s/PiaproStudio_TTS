package main

import "fmt"

func main() {

	fmt.Printf("現在の待ちタスク数: %d\n", q.size)

	// 3. キューが空になるまで取り出して実行 (Front -> Pop)
	for !q.IsEmpty() {
		// 先頭の関数を取得
		task, _ := q.Front()

		// 実行
		task()

		// キューから削除（内部で nil 代入してメモリリークを防止）
		q.Pop()
	}

	fmt.Println("すべてのタスクが完了しました")
}
