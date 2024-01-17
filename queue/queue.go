package queue

import (
	"sync"

	"k8s.io/client-go/util/workqueue"
)

// LengthyQueue is a wrapper around workqueue that keeps track of its length
// Uses sync.mutex for locking on all operations
type LengthyQueue struct {
	queue  *workqueue.Type
	length int
	mu     sync.Mutex
}

// New returns a pointer to a new LengthyQueue of length 0
func New() *LengthyQueue {
	return &LengthyQueue{
		queue:  workqueue.New(),
		length: 0,
	}
}

func (q *LengthyQueue) Get() (interface{}, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.Get()
}

func (q *LengthyQueue) Done(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue.Done(item)
	q.length -= 1
}

func (q *LengthyQueue) Add(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.queue.ShuttingDown() {
		return
	}
	q.queue.Add(item)
	q.length += 1
}

func (q *LengthyQueue) ShutDown() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue.ShutDown()
}

func (q *LengthyQueue) ShuttingDown() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.ShuttingDown()
}

func (q *LengthyQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.length
}
