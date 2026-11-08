package store

import (
	"path/filepath"
	"testing"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
)

func testContract(version string) *contract.Contract {
	return &contract.Contract{
		Name:    "orders",
		Version: version,
		Fields: []contract.Field{
			{Name: "order_id", Type: contract.TypeString, Required: true},
			{Name: "amount", Type: contract.TypeNumber, Required: true},
		},
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestIngestValidEventIsAccepted(t *testing.T) {
	s := openTestStore(t)
	c := testContract("1.0.0")
	res, err := s.Ingest("producer-a", c, map[string]any{"order_id": "o1", "amount": 9.99})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Accepted {
		t.Fatalf("expected acceptance, got violations: %v", res.Violations)
	}
	n, _ := s.CountEvents()
	if n != 1 {
		t.Fatalf("expected 1 event in the store, got %d", n)
	}
}

func TestIngestInvalidEventIsRejectedToDeadLetter(t *testing.T) {
	s := openTestStore(t)
	c := testContract("1.0.0")
	res, err := s.Ingest("producer-a", c, map[string]any{"amount": 9.99}) // missing order_id
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted {
		t.Fatal("expected rejection for a missing required field")
	}
	if n, _ := s.CountEvents(); n != 0 {
		t.Fatalf("expected 0 accepted events, got %d", n)
	}
	if n, _ := s.CountDeadLetters(); n != 1 {
		t.Fatalf("expected 1 dead letter, got %d", n)
	}
}

func TestMultipleProducersIngestIndependently(t *testing.T) {
	s := openTestStore(t)
	c := testContract("1.0.0")
	s.Ingest("producer-a", c, map[string]any{"order_id": "a1", "amount": 1.0})
	s.Ingest("producer-b", c, map[string]any{"order_id": "b1", "amount": 2.0})
	s.Ingest("producer-b", c, map[string]any{"amount": 3.0}) // invalid

	events, _ := s.CountEvents()
	deadLetters, _ := s.CountDeadLetters()
	if events != 2 {
		t.Fatalf("expected 2 accepted events across both producers, got %d", events)
	}
	if deadLetters != 1 {
		t.Fatalf("expected 1 dead letter, got %d", deadLetters)
	}
}

func TestContractVersionsSeenTracksRollingUpgrade(t *testing.T) {
	s := openTestStore(t)
	v1 := testContract("1.0.0")
	v2 := testContract("1.1.0")
	s.Ingest("producer-a", v1, map[string]any{"order_id": "a1", "amount": 1.0})
	s.Ingest("producer-b", v2, map[string]any{"order_id": "b1", "amount": 2.0})

	versions, err := s.ContractVersionsSeen("orders")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0] != "1.0.0" || versions[1] != "1.1.0" {
		t.Fatalf("expected both versions tracked during a rolling upgrade, got %v", versions)
	}
}
