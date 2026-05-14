// queue.go — Bounded message queue with a worker pool.
//
// A message queue holds tasks in a FIFO buffer and distributes them
// to a pool of worker goroutines. Each task is processed by exactly one worker.
//
// Unlike pub-sub (where every subscriber gets a copy), here workers compete:
// only the first free worker grabs each task.
//
//   Producer: queue.Push(task)  → task goes into the buffer
//   Worker:   reads from channel → processes one task at a time
//
// Bounded: the queue has a maximum capacity. If it's full, Push blocks or
// returns an error (depending on which method you use).
//
// Worker pool: goroutines are started in Start() and stopped via Close().

package queue

import (
	"fmt"
	"sync"
)

// Task is one unit of work to be processed by a worker.
//
// ID:      an identifier (e.g., "order-123") for logging
// Payload: the actual data to process (any type)
type Task struct {
	ID      string
	Payload any
}

// HandlerFunc is the function signature for task processors.
// A worker calls this with each task it picks up.
type HandlerFunc func(task Task)

// Queue is a bounded FIFO task queue backed by a Go channel.
type Queue struct {
	ch      chan Task      // buffered channel acts as the FIFO queue
	wg      sync.WaitGroup // tracks running workers
	handler HandlerFunc    // function each worker calls to process a task
	once    sync.Once      // ensures Close is called only once
}

// New creates a queue with the given buffer capacity and worker count.
//
//	capacity: max tasks that can be queued before Push blocks
//	workers:  number of goroutines that process tasks in parallel
//	handler:  function called for each task
//
// Call Start() to launch the workers.
func New(capacity, workers int, handler HandlerFunc) *Queue {
	q := &Queue{
		ch:      make(chan Task, capacity),
		handler: handler,
	}
	q.start(workers)
	return q
}

// start launches `count` worker goroutines that read from the queue channel.
func (q *Queue) start(count int) {
	for range count {
		q.wg.Add(1)
		go func() {
			defer q.wg.Done()
			// range over a channel: reads until the channel is closed
			for task := range q.ch {
				q.handler(task)
			}
		}()
	}
}

// Push adds a task to the queue.
//
// If the queue buffer is full, this blocks until a worker frees space.
// Use TryPush if you want non-blocking behaviour.
func (q *Queue) Push(task Task) {
	q.ch <- task
}

// TryPush attempts to add a task without blocking.
// Returns an error if the queue is full.
func (q *Queue) TryPush(task Task) error {
	select {
	case q.ch <- task:
		return nil
	default:
		return fmt.Errorf("queue is full (capacity %d)", cap(q.ch))
	}
}

// Close stops accepting new tasks and waits for all in-flight tasks to finish.
// After Close, Push will panic (channel closed). Call only once.
func (q *Queue) Close() {
	q.once.Do(func() {
		close(q.ch) // signals workers to stop after draining
		q.wg.Wait() // wait for all workers to finish their current task
	})
}

// Len returns the number of tasks currently waiting in the queue buffer.
// This is an approximation — it can change immediately after reading.
func (q *Queue) Len() int {
	return len(q.ch)
}

// Cap returns the maximum queue capacity.
func (q *Queue) Cap() int {
	return cap(q.ch)
}
