package main

type Queue struct { // fifo
	data []func()
	size int
}

type FuncQ struct {
	*Queue
}

func (q FuncQ) runFuncQ() {
	for !q.IsEmpty() {
		// 先頭の関数を取得
		task, _ := q.Front()
		// 実行
		task()
		// キューから削除（内部で nil 代入してメモリリークを防止）
		q.Pop()
	}
}

// 新規キュー
func NewQueue(cap int) *Queue {
	return &Queue{data: make([]func(), 0, cap), size: 0}
}

// Push追加うしろ
func (q *Queue) Push(n func()) {
	q.data = append(q.data, n)
	q.size++
}

// Pop最初の消す
func (q *Queue) Pop() bool {
	if q.IsEmpty() {
		return false
	}
	q.size--
	q.data[0] = nil
	q.data = q.data[1:]
	return true
}

// Front頭取得
func (q *Queue) Front() (func(), bool) {
	if q.IsEmpty() {
		return nil, false
	}
	return q.data[0], true
}

func (q *Queue) IsEmpty() bool {
	return q.size == 0
}
