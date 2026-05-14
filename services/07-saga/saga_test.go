// saga_test.go — Tests for the Saga orchestrator and compensating transactions.

package saga_test

import (
	"errors"
	"testing"

	"microservices-go/services/07-saga/internal/saga"
	"microservices-go/services/07-saga/internal/services"
)

// ── Orchestrator Unit Tests ───────────────────────────────────────────────────

func TestOrchestrator_AllStepsSucceed(t *testing.T) {
	calls := []string{}

	result := saga.New([]saga.Step{
		{Name: "step1", Action: func() error { calls = append(calls, "step1"); return nil }},
		{Name: "step2", Action: func() error { calls = append(calls, "step2"); return nil }},
		{Name: "step3", Action: func() error { calls = append(calls, "step3"); return nil }},
	}).Run()

	if !result.Succeeded {
		t.Errorf("expected success, got failed: %v", result.Error)
	}
	if len(calls) != 3 {
		t.Errorf("expected 3 steps called, got %v", calls)
	}
	if len(result.Compensated) != 0 {
		t.Errorf("expected no compensations on success, got %v", result.Compensated)
	}
}

func TestOrchestrator_FirstStepFails_NoCompensation(t *testing.T) {
	compCalled := false
	result := saga.New([]saga.Step{
		{
			Name:         "step1",
			Action:       func() error { return errors.New("step1 failed") },
			Compensation: func() error { compCalled = true; return nil },
		},
	}).Run()

	if result.Succeeded {
		t.Error("expected failure")
	}
	if result.FailedStep != "step1" {
		t.Errorf("expected FailedStep='step1', got %q", result.FailedStep)
	}
	// No completed steps before step1, so no compensation needed
	if compCalled {
		t.Error("step1 compensation should NOT run (step1 itself failed)")
	}
	if len(result.Compensated) != 0 {
		t.Errorf("expected empty Compensated list, got %v", result.Compensated)
	}
}

func TestOrchestrator_MiddleStepFails_CompensatesPreviousSteps(t *testing.T) {
	var comp1Called, comp2Called bool

	result := saga.New([]saga.Step{
		{
			Name:         "step1",
			Action:       func() error { return nil },
			Compensation: func() error { comp1Called = true; return nil },
		},
		{
			Name:         "step2",
			Action:       func() error { return nil },
			Compensation: func() error { comp2Called = true; return nil },
		},
		{
			Name:         "step3",
			Action:       func() error { return errors.New("step3 failed") },
			Compensation: func() error { return nil }, // should NOT run (step3 failed, not completed)
		},
	}).Run()

	if result.Succeeded {
		t.Error("expected failure")
	}
	if result.FailedStep != "step3" {
		t.Errorf("expected FailedStep='step3', got %q", result.FailedStep)
	}
	if !comp1Called {
		t.Error("step1 compensation should have run")
	}
	if !comp2Called {
		t.Error("step2 compensation should have run")
	}
}

func TestOrchestrator_CompensationRunsInReverseOrder(t *testing.T) {
	var order []string

	saga.New([]saga.Step{
		{
			Name:         "step1",
			Action:       func() error { return nil },
			Compensation: func() error { order = append(order, "comp1"); return nil },
		},
		{
			Name:         "step2",
			Action:       func() error { return nil },
			Compensation: func() error { order = append(order, "comp2"); return nil },
		},
		{
			Name:   "step3",
			Action: func() error { return errors.New("fail") },
		},
	}).Run()

	if len(order) != 2 {
		t.Fatalf("expected 2 compensations, got %v", order)
	}
	// Compensation should run in reverse: step2 first, then step1
	if order[0] != "comp2" || order[1] != "comp1" {
		t.Errorf("expected [comp2 comp1] order, got %v", order)
	}
}

func TestOrchestrator_CompensationErrorRecorded(t *testing.T) {
	result := saga.New([]saga.Step{
		{
			Name:         "step1",
			Action:       func() error { return nil },
			Compensation: func() error { return errors.New("refund failed: bank down") },
		},
		{
			Name:   "step2",
			Action: func() error { return errors.New("step2 failed") },
		},
	}).Run()

	if result.Succeeded {
		t.Error("expected failure")
	}
	if len(result.CompErrors) == 0 {
		t.Error("expected compensation error to be recorded")
	}
	// Despite the compensation error, the failed step should be recorded
	if result.FailedStep != "step2" {
		t.Errorf("expected FailedStep='step2', got %q", result.FailedStep)
	}
}

func TestOrchestrator_NoSteps_Succeeds(t *testing.T) {
	result := saga.New([]saga.Step{}).Run()
	if !result.Succeeded {
		t.Error("empty saga should succeed")
	}
}

// ── Service Tests (Payment, Inventory, Shipping) ──────────────────────────────

func TestPaymentService_ChargeAndRefund(t *testing.T) {
	p := services.NewPaymentService()

	if err := p.Charge("ord-1", 5000); err != nil {
		t.Fatalf("charge error: %v", err)
	}
	if !p.IsCharged("ord-1") {
		t.Error("expected order to be charged")
	}

	if err := p.Refund("ord-1"); err != nil {
		t.Fatalf("refund error: %v", err)
	}
	if p.IsCharged("ord-1") {
		t.Error("expected order to no longer be charged after refund")
	}
}

func TestPaymentService_FailNext(t *testing.T) {
	p := services.NewPaymentService()
	p.FailNext()

	err := p.Charge("ord-1", 5000)
	if err == nil {
		t.Error("expected charge to fail after FailNext()")
	}

	// Subsequent calls should succeed again
	if err := p.Charge("ord-1", 5000); err != nil {
		t.Errorf("expected success after FailNext() was consumed, got: %v", err)
	}
}

func TestInventoryService_ReserveAndRelease(t *testing.T) {
	inv := services.NewInventoryService()

	if err := inv.Reserve("ord-1", 3); err != nil {
		t.Fatalf("reserve error: %v", err)
	}
	if !inv.IsReserved("ord-1") {
		t.Error("expected reservation to exist")
	}

	if err := inv.Release("ord-1"); err != nil {
		t.Fatalf("release error: %v", err)
	}
	if inv.IsReserved("ord-1") {
		t.Error("expected reservation to be gone after release")
	}
}

func TestShippingService_CreateAndCancel(t *testing.T) {
	s := services.NewShippingService()

	tracking, err := s.Create("ord-1", "123 Main St")
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if tracking == "" {
		t.Error("expected non-empty tracking number")
	}
	if !s.IsShipped("ord-1") {
		t.Error("expected shipment to exist")
	}

	if err := s.Cancel("ord-1"); err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if s.IsShipped("ord-1") {
		t.Error("expected shipment to be gone after cancel")
	}
}

// ── Full Saga Integration Tests ───────────────────────────────────────────────

func buildOrderSaga(
	orderID string,
	payment *services.PaymentService,
	inventory *services.InventoryService,
	shipping *services.ShippingService,
) *saga.Orchestrator {
	return saga.New([]saga.Step{
		{
			Name:         "Payment.Charge",
			Action:       func() error { return payment.Charge(orderID, 5000) },
			Compensation: func() error { return payment.Refund(orderID) },
		},
		{
			Name:         "Inventory.Reserve",
			Action:       func() error { return inventory.Reserve(orderID, 1) },
			Compensation: func() error { return inventory.Release(orderID) },
		},
		{
			Name:         "Shipping.Create",
			Action:       func() error { _, err := shipping.Create(orderID, "123 Main St"); return err },
			Compensation: func() error { return shipping.Cancel(orderID) },
		},
	})
}

func TestSaga_HappyPath(t *testing.T) {
	p := services.NewPaymentService()
	inv := services.NewInventoryService()
	s := services.NewShippingService()

	result := buildOrderSaga("ord-happy", p, inv, s).Run()

	if !result.Succeeded {
		t.Fatalf("expected success, got: %v", result.Error)
	}
	if !p.IsCharged("ord-happy") {
		t.Error("payment should be charged")
	}
	if !inv.IsReserved("ord-happy") {
		t.Error("inventory should be reserved")
	}
	if !s.IsShipped("ord-happy") {
		t.Error("shipment should exist")
	}
}

func TestSaga_ShippingFails_CompensatesPaymentAndInventory(t *testing.T) {
	p := services.NewPaymentService()
	inv := services.NewInventoryService()
	s := services.NewShippingService()
	s.FailNext()

	result := buildOrderSaga("ord-fail", p, inv, s).Run()

	if result.Succeeded {
		t.Fatal("expected failure")
	}
	if result.FailedStep != "Shipping.Create" {
		t.Errorf("expected FailedStep='Shipping.Create', got %q", result.FailedStep)
	}

	// Payment and inventory should have been compensated
	if p.IsCharged("ord-fail") {
		t.Error("payment should have been refunded (compensated)")
	}
	if inv.IsReserved("ord-fail") {
		t.Error("inventory should have been released (compensated)")
	}

	// Both compensations should be listed
	if len(result.Compensated) != 2 {
		t.Errorf("expected 2 compensations (Payment + Inventory), got %v", result.Compensated)
	}
}

func TestSaga_InventoryFails_CompensatesPaymentOnly(t *testing.T) {
	p := services.NewPaymentService()
	inv := services.NewInventoryService()
	s := services.NewShippingService()
	inv.FailNext()

	result := buildOrderSaga("ord-inv-fail", p, inv, s).Run()

	if result.Succeeded {
		t.Fatal("expected failure")
	}

	// Only payment compensation should run (inventory failed, shipping not reached)
	if p.IsCharged("ord-inv-fail") {
		t.Error("payment should have been refunded")
	}
	if len(result.Compensated) != 1 {
		t.Errorf("expected 1 compensation (Payment only), got %v", result.Compensated)
	}
}

func TestSaga_PaymentFails_NoCompensation(t *testing.T) {
	p := services.NewPaymentService()
	inv := services.NewInventoryService()
	s := services.NewShippingService()
	p.FailNext()

	result := buildOrderSaga("ord-pay-fail", p, inv, s).Run()

	if result.Succeeded {
		t.Fatal("expected failure")
	}

	// No completed steps before payment → no compensations
	if len(result.Compensated) != 0 {
		t.Errorf("expected no compensations when first step fails, got %v", result.Compensated)
	}
}
