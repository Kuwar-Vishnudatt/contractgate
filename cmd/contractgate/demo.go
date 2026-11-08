package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/compat"
	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
	"github.com/Kuwar-Vishnudatt/contractgate/internal/store"
)

func runDemo(args []string) {
	dbPath := filepath.Join(os.TempDir(), "contractgate-demo.db")
	os.Remove(dbPath)

	s, err := store.Open(dbPath)
	must(err)
	defer s.Close()

	v1, err := contract.Load("contracts/orders/v1.yaml")
	must(err)
	v2Compatible, err := contract.Load("contracts/orders/v2-compatible.yaml")
	must(err)
	v2Breaking, err := contract.Load("contracts/orders/v2-breaking.yaml")
	must(err)

	fmt.Println("=== 1. CI gate: v1 -> v2-compatible (adds an optional field) ===")
	printDecision(must2(compat.Evaluate(v1, v2Compatible)))

	fmt.Println("\n=== 2. CI gate: v1 -> v2-breaking (removes a required field, no major bump) ===")
	printDecision(must2(compat.Evaluate(v1, v2Breaking)))

	fmt.Println("\n=== 3. Multi-producer ingestion against the current contract (v1) ===")
	r1, _ := s.Ingest("checkout-service", v1, map[string]any{"order_id": "o1", "amount": 9.99, "status": "placed"})
	fmt.Printf("checkout-service event: accepted=%v\n", r1.Accepted)

	r2, _ := s.Ingest("legacy-importer", v1, map[string]any{"amount": 5.0, "status": "placed"}) // missing order_id
	fmt.Printf("legacy-importer event (missing order_id): accepted=%v violations=%v\n", r2.Accepted, r2.Violations)

	fmt.Println("\n=== 4. Rolling upgrade: one producer moves to v2-compatible while another stays on v1 ===")
	s.Ingest("checkout-service", v2Compatible, map[string]any{"order_id": "o2", "amount": 12.5, "status": "placed", "discount_code": "SAVE10"})
	s.Ingest("legacy-importer", v1, map[string]any{"order_id": "o3", "amount": 3.0, "status": "shipped"})
	versions, _ := s.ContractVersionsSeen("orders")
	fmt.Printf("contract versions concurrently in flight: %v\n", versions)

	accepted, _ := s.CountEvents()
	rejected, _ := s.CountDeadLetters()
	fmt.Printf("\ntotals: accepted=%d rejected-to-dead-letter=%d\n", accepted, rejected)
}

func printDecision(d compat.Decision) {
	if len(d.Report.BackwardIssues) > 0 {
		fmt.Println("  backward issues:")
		for _, i := range d.Report.BackwardIssues {
			fmt.Printf("    - %s\n", i.Message)
		}
	}
	if len(d.Report.ForwardIssues) > 0 {
		fmt.Println("  forward issues:")
		for _, i := range d.Report.ForwardIssues {
			fmt.Printf("    - %s\n", i.Message)
		}
	}
	if d.Blocked {
		fmt.Println("  RESULT: BLOCKED")
		for _, r := range d.Reasons {
			fmt.Println("    -", r)
		}
	} else {
		fmt.Println("  RESULT: OK")
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func must2[T any](v T, err error) T {
	must(err)
	return v
}
