// shipping.go — Fake Shipping service for the Saga demo.

package services

import (
	"errors"
	"fmt"
	"sync"
)

// ShippingService simulates creating and cancelling shipments.
type ShippingService struct {
	mu        sync.Mutex
	shipments map[string]string // orderID → tracking number
	failNext  bool
}

// NewShippingService creates a shipping service.
func NewShippingService() *ShippingService {
	return &ShippingService{shipments: make(map[string]string)}
}

// FailNext makes the next Create call fail.
func (s *ShippingService) FailNext() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = true
}

// Create books a shipment for the order and returns a tracking number.
func (s *ShippingService) Create(orderID, address string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failNext {
		s.failNext = false
		return "", errors.New("shipping: carrier unavailable")
	}
	if address == "" {
		return "", fmt.Errorf("address is required")
	}

	tracking := "TRACK-" + orderID
	s.shipments[orderID] = tracking
	return tracking, nil
}

// Cancel undoes a shipment — the compensating action for Create.
func (s *ShippingService) Cancel(orderID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.shipments[orderID]; !exists {
		return fmt.Errorf("no shipment found for order %q", orderID)
	}

	delete(s.shipments, orderID)
	return nil
}

// IsShipped returns true if the order has an active shipment.
func (s *ShippingService) IsShipped(orderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.shipments[orderID]
	return ok
}
