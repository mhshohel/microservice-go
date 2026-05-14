// inventory.go — Fake Inventory service for the Saga demo.

package services

import (
	"errors"
	"fmt"
	"sync"
)

// InventoryService simulates reserving and releasing stock.
type InventoryService struct {
	mu       sync.Mutex
	reserved map[string]int // orderID → quantity reserved
	failNext bool
}

// NewInventoryService creates an inventory service.
func NewInventoryService() *InventoryService {
	return &InventoryService{reserved: make(map[string]int)}
}

// FailNext makes the next Reserve call fail.
func (inv *InventoryService) FailNext() {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.failNext = true
}

// Reserve holds the given quantity for the order.
func (inv *InventoryService) Reserve(orderID string, qty int) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	if inv.failNext {
		inv.failNext = false
		return errors.New("inventory: out of stock")
	}
	if qty <= 0 {
		return fmt.Errorf("quantity must be > 0, got %d", qty)
	}

	inv.reserved[orderID] = qty
	return nil
}

// Release undoes a reservation — the compensating action for Reserve.
func (inv *InventoryService) Release(orderID string) error {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	if _, exists := inv.reserved[orderID]; !exists {
		return fmt.Errorf("no reservation found for order %q", orderID)
	}

	delete(inv.reserved, orderID)
	return nil
}

// IsReserved returns true if the order has an active reservation.
func (inv *InventoryService) IsReserved(orderID string) bool {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	_, ok := inv.reserved[orderID]
	return ok
}
