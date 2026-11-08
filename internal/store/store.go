// Package store is the runtime ingestion gateway multiple producers write
// through. Every event is validated against its producer's current
// contract before being accepted; a violating event is rejected (and
// recorded in a dead-letter table for inspection) rather than silently
// corrupting the shared table with data that doesn't conform to the
// contract consumers were promised.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Kuwar-Vishnudatt/contractgate/internal/contract"
	"github.com/Kuwar-Vishnudatt/contractgate/internal/validate"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single-writer SQLite: avoid concurrent-write lock errors

	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		producer TEXT NOT NULL,
		contract_name TEXT NOT NULL,
		contract_version TEXT NOT NULL,
		payload TEXT NOT NULL,
		ingested_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS dead_letters (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		producer TEXT NOT NULL,
		contract_name TEXT NOT NULL,
		payload TEXT NOT NULL,
		violations TEXT NOT NULL,
		rejected_at TEXT NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// IngestResult reports what happened to one submitted event.
type IngestResult struct {
	Accepted   bool
	Violations []validate.Violation
}

// Ingest validates data against c and, if it conforms, commits it to the
// shared events table tagged with the producer and the exact contract
// version it was validated against (so a later audit can tell which
// contract version was in force when any given row was accepted). A
// violating event is routed to the dead-letter table instead of being
// silently dropped or, worse, accepted anyway.
func (s *Store) Ingest(producer string, c *contract.Contract, data map[string]any) (IngestResult, error) {
	violations := validate.Event(c, data)
	payload, err := json.Marshal(data)
	if err != nil {
		return IngestResult{}, fmt.Errorf("marshal payload: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if len(violations) > 0 {
		vJSON, _ := json.Marshal(violations)
		_, err := s.db.Exec(
			`INSERT INTO dead_letters (producer, contract_name, payload, violations, rejected_at) VALUES (?, ?, ?, ?, ?)`,
			producer, c.Name, string(payload), string(vJSON), now,
		)
		if err != nil {
			return IngestResult{}, fmt.Errorf("record dead letter: %w", err)
		}
		return IngestResult{Accepted: false, Violations: violations}, nil
	}

	_, err = s.db.Exec(
		`INSERT INTO events (producer, contract_name, contract_version, payload, ingested_at) VALUES (?, ?, ?, ?, ?)`,
		producer, c.Name, c.Version, string(payload), now,
	)
	if err != nil {
		return IngestResult{}, fmt.Errorf("insert event: %w", err)
	}
	return IngestResult{Accepted: true}, nil
}

func (s *Store) CountEvents() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&n)
	return n, err
}

func (s *Store) CountDeadLetters() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM dead_letters`).Scan(&n)
	return n, err
}

// ContractVersionsSeen returns the distinct contract versions recorded
// against accepted events for contractName, useful for confirming a
// multi-producer rollout: during a rolling upgrade, more than one version
// can legitimately be in flight at once.
func (s *Store) ContractVersionsSeen(contractName string) ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT contract_version FROM events WHERE contract_name = ? ORDER BY contract_version`, contractName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
