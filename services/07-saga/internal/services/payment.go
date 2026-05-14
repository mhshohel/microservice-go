// payment.go — Fake Payment service for the Saga demo.
//
// In a real system this would call a payments API (Stripe, etc.).
// Here we just track state in memory to demonstrate the saga flow.

package services

import (
	"errors"
	"fmt"
	"sync"
)

// PaymentService simulates charging and refunding a customer.
type PaymentService struct {
	mu       sync.Mutex
	charged  map[string]int // orderID → amount in cents
	failNext bool           // if true, the next Charge call will fail
}

// NewPaymentService creates a payment service.
func NewPaymentService() *PaymentService {
	return &PaymentService{charged: make(map[string]int)}
}

// FailNext makes the next Charge call return an error (for testing failure paths).
func (p *PaymentService) FailNext() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNext = true
}

// Charge debits amountCents from the customer for the given order.
func (p *PaymentService) Charge(orderID string, amountCents int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failNext {
		p.failNext = false
		return errors.New("payment declined: card rejected")
	}
	if amountCents <= 0 {
		return fmt.Errorf("invalid amount: %d", amountCents)
	}

	p.charged[orderID] = amountCents
	return nil
}

// Refund reverses a charge — the compensating action for Charge.
func (p *PaymentService) Refund(orderID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.charged[orderID]; !exists {
		return fmt.Errorf("no charge found for order %q", orderID)
	}

	delete(p.charged, orderID)
	return nil
}

// IsCharged returns true if the order has been charged (not refunded).
func (p *PaymentService) IsCharged(orderID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.charged[orderID]
	return ok
}
