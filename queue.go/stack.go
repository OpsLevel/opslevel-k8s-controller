package queue

import (
	"context"
	"sync"
)

// Queue is a FIFO data structure that is thread safe because it locks on every transaction.
// The Queue can become expired which will prevent transactions from working.
type Queue struct {
	items   []any
	mu      sync.Mutex
	expired bool
}

// Returns the pointer to a new Queue.
func New() *Queue {
	return &Queue{
		items: []any{},
	}
}

// Pop is a transaction will block and wait until the Queue has an item that can be returned.
// The boolean return value indicates if the Queue has expired. If this is true the goroutine using the Queue should close.
// Pop will return a nil item if the context passed cancels or if the queue is expired.
func (s *Queue) Pop(ctx context.Context) (any, bool) {
	for {
		// This loop will lock each time it runs. EVERY EXIT FROM THE LOOP NEEDS TO GIVE UP THE LOCK.
		s.mu.Lock()
		isExpired := s.expired // This needs to be read BEFORE unlocking.

		if ctx != nil {
			select {
			case <-ctx.Done():
				s.mu.Unlock()
				return nil, isExpired
			default:
			}
		}

		// if Queue has expired - unlock and exit
		if isExpired {
			s.mu.Unlock()
			return nil, true
		}

		// if Queue is empty - unlock and wait
		if len(s.items) == 0 {
			s.mu.Unlock()
			continue
		} else {
			value := s.items[0]
			s.items = s.items[1:]
			s.mu.Unlock()
			return value, false
		}
		panic("the queue loop is broken") // should not be reachable
	}
}

// Insert is a transaction that will add an item onto the end of the Queue.
// Insert will do nothing if the Queue is expired.
func (s *Queue) Insert(item any) {
	defer s.mu.Unlock()
	s.mu.Lock()
	if s.expired {
		// do nothing
		return
	}
	s.items = append(s.items, item)
}

func (s *Queue) Len() int {
	defer s.mu.Unlock()
	s.mu.Lock()
	return len(s.items)
}

func (s *Queue) SetExpired() {
	defer s.mu.Unlock()
	s.mu.Lock()
	s.expired = true
}

func (s *Queue) IsExpired() bool {
	defer s.mu.Unlock()
	s.mu.Lock()
	return s.expired
}
