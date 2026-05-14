// broker.go — Topic-based publish-subscribe broker using Go channels.
//
// A broker manages topics. Each topic has a list of subscribers.
// When a publisher sends an event on a topic, every subscriber receives a copy.
//
// Each subscriber gets its own buffered channel so a slow subscriber doesn't
// block the publisher or other subscribers (non-blocking fan-out).
//
// Thread safety: subscribe/unsubscribe and publish all use a sync.RWMutex.
// Subscribing modifies the subscriber map → write lock.
// Publishing reads the subscriber map → read lock (allows concurrent publishes).

package pubsub

import (
	"fmt"
	"sync"
)

// Event is a message published to a topic.
//
// Topic: which topic this event belongs to (e.g., "order.placed")
// Payload: the data — can be anything (string, struct, map, etc.)
type Event struct {
	Topic   string
	Payload any
}

// Subscriber represents one consumer subscribed to a topic.
// It receives events through its Ch channel.
type Subscriber struct {
	id string     // unique ID for this subscription
	Ch chan Event // caller reads events from this channel
}

// Broker manages topics and their subscribers.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[string][]*Subscriber // topic → list of subscribers
	bufSize     int                      // channel buffer size per subscriber
}

// New creates a broker.
// bufSize is how many events each subscriber's channel can hold without blocking.
// If a subscriber's buffer is full, Publish drops the event for that subscriber.
func New(bufSize int) *Broker {
	return &Broker{
		subscribers: make(map[string][]*Subscriber),
		bufSize:     bufSize,
	}
}

// Subscribe registers a new subscriber on the given topic.
// Returns a Subscriber whose Ch field receives events on that topic.
//
// Call Unsubscribe when done to free the channel.
func (b *Broker) Subscribe(topic string) *Subscriber {
	sub := &Subscriber{
		id: fmt.Sprintf("%s-%p", topic, new(int)), // unique pointer as ID
		Ch: make(chan Event, b.bufSize),
	}

	b.mu.Lock()
	b.subscribers[topic] = append(b.subscribers[topic], sub)
	b.mu.Unlock()

	return sub
}

// Unsubscribe removes a subscriber from its topic and closes its channel.
// The subscriber should stop reading from Ch after calling Unsubscribe.
func (b *Broker) Unsubscribe(topic string, sub *Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[topic]
	for i, s := range subs {
		if s == sub {
			// Remove this subscriber from the slice
			b.subscribers[topic] = append(subs[:i], subs[i+1:]...)
			close(sub.Ch) // signal the subscriber that no more events will come
			return
		}
	}
}

// Publish sends an event to all subscribers on the given topic.
//
// Fan-out: every subscriber receives a copy of the event.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that subscriber.
// This prevents a slow subscriber from blocking the publisher.
func (b *Broker) Publish(topic string, payload any) {
	event := Event{Topic: topic, Payload: payload}

	b.mu.RLock() // read lock — allows concurrent publishes
	subs := b.subscribers[topic]
	b.mu.RUnlock()

	for _, sub := range subs {
		// Non-blocking send: if the channel is full, skip this subscriber
		// rather than blocking the whole publish call.
		select {
		case sub.Ch <- event:
			// event delivered
		default:
			// subscriber is too slow — drop the event for this subscriber
			// In a production system you'd log a metric here
		}
	}
}

// SubscriberCount returns how many subscribers are on a topic.
// Useful for tests.
func (b *Broker) SubscriberCount(topic string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[topic])
}
