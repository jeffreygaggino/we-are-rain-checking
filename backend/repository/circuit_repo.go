// Package repository is SQL and nothing else: one file per table, every method taking the pool or a
// transaction so a caller can make several writes atomic without changing a method here.
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// CircuitRepo reads the seeded Circuits. There is no write method: this table is owned by the seed
// migration, and ingest resolves against it rather than adding to it (ADR-0003).
type CircuitRepo struct{}

func NewCircuitRepo() *CircuitRepo { return &CircuitRepo{} }

// KeysToIDs loads the whole table as circuit_key -> id.
//
// One query rather than a lookup per Meeting: 26 rows, read once at the top of a run, and every
// resolution after that is a map hit. It is also what lets ingest resolve a whole season before
// opening its transaction.
func (r *CircuitRepo) KeysToIDs(ctx context.Context, db sqlx.ExtContext) (map[int]uuid.UUID, error) {
	const q = `SELECT circuit_key, id FROM f1.circuits`

	rows, err := db.QueryxContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("circuit_repo.KeysToIDs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make(map[int]uuid.UUID)
	for rows.Next() {
		var (
			key int
			id  uuid.UUID
		)
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("circuit_repo.KeysToIDs scan: %w", err)
		}
		ids[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("circuit_repo.KeysToIDs: %w", err)
	}
	return ids, nil
}
