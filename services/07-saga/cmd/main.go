// main.go — Saga Pattern demo.
//
// Demonstrates:
//   1. Happy path: all 3 steps succeed
//   2. Failure path: shipping fails, payment and inventory are compensated
//
// HOW TO RUN:
//   go run ./services/07-saga/cmd/main.go

package main

import (
	"fmt"
	"log/slog"
	"os"

	"microservices-go/services/07-saga/internal/saga"
	"microservices-go/services/07-saga/internal/services"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	payment := services.NewPaymentService()
	inventory := services.NewInventoryService()
	shipping := services.NewShippingService()

	// ── Demo 1: Happy Path ────────────────────────────────────────────────────
	fmt.Println("\n═══════════════════════════════════")
	fmt.Println(" SAGA 1: Happy Path")
	fmt.Println("═══════════════════════════════════")

	result := runOrderSaga("ord-001", payment, inventory, shipping)
	printResult(result)

	// ── Demo 2: Failure Path (shipping fails) ─────────────────────────────────
	fmt.Println("\n═══════════════════════════════════")
	fmt.Println(" SAGA 2: Shipping Fails → Compensate")
	fmt.Println("═══════════════════════════════════")

	shipping.FailNext() // make the next Create call fail
	result = runOrderSaga("ord-002", payment, inventory, shipping)
	printResult(result)

	fmt.Printf("\n  Payment charged for ord-002?  %v (should be false — refunded)\n",
		payment.IsCharged("ord-002"))
	fmt.Printf("  Inventory reserved for ord-002? %v (should be false — released)\n",
		inventory.IsReserved("ord-002"))
}

// runOrderSaga builds and runs the three-step order saga.
func runOrderSaga(
	orderID string,
	payment *services.PaymentService,
	inventory *services.InventoryService,
	shipping *services.ShippingService,
) saga.Result {
	var trackingNumber string

	steps := []saga.Step{
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
			Name: "Shipping.Create",
			Action: func() error {
				var err error
				trackingNumber, err = shipping.Create(orderID, "123 Main St")
				return err
			},
			Compensation: func() error { return shipping.Cancel(orderID) },
		},
	}

	orchestrator := saga.New(steps)
	result := orchestrator.Run()

	if result.Succeeded {
		slog.Info("saga succeeded", "order", orderID, "tracking", trackingNumber)
	} else {
		slog.Error("saga failed", "order", orderID, "failed_step", result.FailedStep, "error", result.Error)
	}

	return result
}

func printResult(r saga.Result) {
	if r.Succeeded {
		fmt.Println("  ✓ All steps completed successfully")
		return
	}
	fmt.Printf("  ✗ Failed at step: %s\n", r.FailedStep)
	fmt.Printf("    Error: %v\n", r.Error)
	if len(r.Compensated) > 0 {
		fmt.Printf("  ↩ Compensated: %v\n", r.Compensated)
	}
	if len(r.CompErrors) > 0 {
		fmt.Printf("  ! Compensation errors: %v\n", r.CompErrors)
	}
}
