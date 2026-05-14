// events.go — Event types and a simple in-process event bus.
//
// Events are the bridge between the write side and the read side.
// When the write side changes data, it publishes an event.
// The read side listens and updates its denormalized view.
//
// This is a synchronous in-process bus using a Go channel.
// In production you'd use Kafka, RabbitMQ, or NATS instead.

package events

// EventType identifies what happened.
type EventType string

const (
	OrderPlaced  EventType = "OrderPlaced"
	OrderShipped EventType = "OrderShipped"
)

// Event carries the type and payload of something that happened.
type Event struct {
	Type    EventType
	Payload any // the specific event data — cast to the concrete type
}

// OrderPlacedPayload is the data for an OrderPlaced event.
type OrderPlacedPayload struct {
	OrderID    string
	CustomerID string
	Customer   string
	Item       string
	Quantity   int
	TotalCents int // price in cents to avoid float precision issues
}

// OrderShippedPayload is the data for an OrderShipped event.
type OrderShippedPayload struct {
	OrderID        string
	TrackingNumber string
}

// Bus is a simple in-process event bus backed by a buffered channel.
// Publishers call Publish; the projector reads via Subscribe.
type Bus struct {
	ch chan Event
}

// NewBus creates a bus with the given channel buffer size.
func NewBus(bufSize int) *Bus {
	return &Bus{ch: make(chan Event, bufSize)}
}

// Publish sends an event to the bus. Non-blocking: if the buffer is full, drops the event.
// In production you'd handle backpressure differently (retry, dead-letter queue).
func (b *Bus) Publish(e Event) {
	select {
	case b.ch <- e:
	default:
		// bus is full — in production: log a metric, retry, or use a larger buffer
	}
}

// Subscribe returns a read-only channel.
// The caller reads events from this channel (typically in a goroutine).
func (b *Bus) Subscribe() <-chan Event {
	return b.ch
}

// Close signals that no more events will be published.
func (b *Bus) Close() {
	close(b.ch)
}
