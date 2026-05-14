// event_driven_test.go — Tests for pub-sub broker and message queue.
//
// Pub-sub tests:
//   - Event is delivered to ALL subscribers on a topic
//   - Subscribers on different topics are isolated
//   - Unsubscribe stops delivery to that subscriber
//   - Multiple publishers can publish concurrently
//
// Queue tests:
//   - Tasks are processed (each task processed exactly once)
//   - TryPush returns error when queue is full
//   - Close drains the queue before returning
//   - Multiple workers share the load

package event_driven_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"microservices-go/services/05-event-driven/internal/pubsub"
	"microservices-go/services/05-event-driven/internal/queue"
)

// ── Pub-Sub Tests ─────────────────────────────────────────────────────────────

func TestPubSub_EventDeliveredToAllSubscribers(t *testing.T) {
	broker := pubsub.New(10)

	// Subscribe 3 consumers to the same topic
	sub1 := broker.Subscribe("order.placed")
	sub2 := broker.Subscribe("order.placed")
	sub3 := broker.Subscribe("order.placed")

	// Publish one event
	broker.Publish("order.placed", "order-123")

	// All 3 should receive the event
	for i, sub := range []*pubsub.Subscriber{sub1, sub2, sub3} {
		select {
		case event := <-sub.Ch:
			if event.Payload != "order-123" {
				t.Errorf("subscriber %d: expected 'order-123', got %v", i+1, event.Payload)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d: timed out waiting for event", i+1)
		}
	}
}

func TestPubSub_TopicsAreIsolated(t *testing.T) {
	broker := pubsub.New(10)

	orderSub := broker.Subscribe("order.placed")
	userSub := broker.Subscribe("user.created")

	// Publish to "order.placed"
	broker.Publish("order.placed", "order-data")

	// orderSub should receive it
	select {
	case event := <-orderSub.Ch:
		if event.Payload != "order-data" {
			t.Errorf("expected 'order-data', got %v", event.Payload)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("orderSub: timed out waiting for event")
	}

	// userSub should NOT receive it (different topic)
	select {
	case event := <-userSub.Ch:
		t.Errorf("userSub should not receive order events, but got: %v", event)
	case <-time.After(20 * time.Millisecond):
		// correct — nothing received
	}
}

func TestPubSub_Unsubscribe_StopsDelivery(t *testing.T) {
	broker := pubsub.New(10)

	sub := broker.Subscribe("test.topic")

	// Unsubscribe immediately
	broker.Unsubscribe("test.topic", sub)

	// Subscriber count should be 0 now
	if broker.SubscriberCount("test.topic") != 0 {
		t.Errorf("expected 0 subscribers after unsubscribe, got %d", broker.SubscriberCount("test.topic"))
	}

	// The channel should be closed — read returns zero value, ok=false
	select {
	case _, ok := <-sub.Ch:
		if ok {
			t.Error("expected channel to be closed after Unsubscribe")
		}
		// channel closed, ok=false → correct
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for channel close")
	}
}

func TestPubSub_MultiplePublishers_ConcurrentSafe(t *testing.T) {
	broker := pubsub.New(100)
	sub := broker.Subscribe("concurrent.topic")

	var received atomic.Int64

	// Reader goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range sub.Ch {
			received.Add(1)
		}
	}()

	// 10 concurrent publishers each send 5 events = 50 total
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				broker.Publish("concurrent.topic", "ping")
			}
		}()
	}
	wg.Wait()

	// Unsubscribe closes the channel, which ends the reader goroutine
	broker.Unsubscribe("concurrent.topic", sub)
	<-done // wait for reader to drain

	if received.Load() != 50 {
		t.Errorf("expected 50 events, received %d", received.Load())
	}
}

func TestPubSub_NoSubscribers_PublishDoesNotPanic(t *testing.T) {
	broker := pubsub.New(10)
	// Publish with no subscribers — should not panic
	broker.Publish("nobody.listening", "data")
}

func TestPubSub_SubscriberCount(t *testing.T) {
	broker := pubsub.New(10)

	if broker.SubscriberCount("topic") != 0 {
		t.Error("expected 0 subscribers on new broker")
	}

	sub1 := broker.Subscribe("topic")
	sub2 := broker.Subscribe("topic")

	if broker.SubscriberCount("topic") != 2 {
		t.Errorf("expected 2 subscribers, got %d", broker.SubscriberCount("topic"))
	}

	broker.Unsubscribe("topic", sub1)
	if broker.SubscriberCount("topic") != 1 {
		t.Errorf("expected 1 subscriber after unsubscribe, got %d", broker.SubscriberCount("topic"))
	}

	broker.Unsubscribe("topic", sub2)
	if broker.SubscriberCount("topic") != 0 {
		t.Errorf("expected 0 subscribers after second unsubscribe, got %d", broker.SubscriberCount("topic"))
	}
}

// ── Message Queue Tests ───────────────────────────────────────────────────────

func TestQueue_AllTasksProcessed(t *testing.T) {
	var processed atomic.Int64

	q := queue.New(20, 3, func(task queue.Task) {
		processed.Add(1)
	})

	const taskCount = 15
	for i := range taskCount {
		q.Push(queue.Task{ID: fmt.Sprintf("task-%d", i)})
	}

	q.Close()

	if processed.Load() != taskCount {
		t.Errorf("expected %d tasks processed, got %d", taskCount, processed.Load())
	}
}

func TestQueue_EachTaskProcessedExactlyOnce(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}

	q := queue.New(20, 5, func(task queue.Task) {
		mu.Lock()
		seen[task.ID]++
		mu.Unlock()
	})

	for i := range 10 {
		q.Push(queue.Task{ID: fmt.Sprintf("task-%d", i)})
	}
	q.Close()

	// Every task should appear exactly once
	for id, count := range seen {
		if count != 1 {
			t.Errorf("task %s was processed %d times (expected 1)", id, count)
		}
	}
	if len(seen) != 10 {
		t.Errorf("expected 10 unique tasks, got %d", len(seen))
	}
}

func TestQueue_TryPush_FailsWhenFull(t *testing.T) {
	// Create a queue with capacity=2 but workers that block (so buffer fills fast)
	blocked := make(chan struct{})
	q := queue.New(2, 1, func(_ queue.Task) {
		<-blocked // block workers so the queue fills up
	})

	// Fill the queue: 1 is being processed, 2 in buffer
	q.Push(queue.Task{ID: "task-1"})
	q.Push(queue.Task{ID: "task-2"})

	// Give worker time to pick up task-1
	time.Sleep(5 * time.Millisecond)

	// Fill the buffer
	q.TryPush(queue.Task{ID: "task-3"}) //nolint — may or may not succeed

	// Now queue should be full — TryPush should fail
	err := q.TryPush(queue.Task{ID: "task-overflow"})
	if err == nil {
		t.Error("expected error when pushing to full queue, got nil")
	}

	close(blocked)
	q.Close()
}

func TestQueue_CloseWaitsForTasksToFinish(t *testing.T) {
	var completed atomic.Int64

	q := queue.New(20, 2, func(task queue.Task) {
		time.Sleep(10 * time.Millisecond) // simulate work
		completed.Add(1)
	})

	for i := range 5 {
		q.Push(queue.Task{ID: fmt.Sprintf("task-%d", i)})
	}

	// Close should block until all 5 tasks are done
	q.Close()

	if completed.Load() != 5 {
		t.Errorf("Close returned before all tasks finished: completed=%d", completed.Load())
	}
}

func TestQueue_Len_ReflectsBufferSize(t *testing.T) {
	// Workers that block so we can inspect the queue length
	blocked := make(chan struct{})
	q := queue.New(10, 1, func(_ queue.Task) {
		<-blocked
	})

	// Push 3 tasks. The first goes to the worker, 2 stay in the buffer.
	q.Push(queue.Task{ID: "t1"})
	q.Push(queue.Task{ID: "t2"})
	q.Push(queue.Task{ID: "t3"})

	// Give worker time to dequeue the first task
	time.Sleep(5 * time.Millisecond)

	l := q.Len()
	if l < 1 {
		t.Errorf("expected at least 1 task in queue buffer, got %d", l)
	}

	close(blocked)
	q.Close()
}

func TestQueue_Cap_IsCorrect(t *testing.T) {
	q := queue.New(42, 1, func(_ queue.Task) {})
	if q.Cap() != 42 {
		t.Errorf("expected capacity 42, got %d", q.Cap())
	}
	q.Close()
}
