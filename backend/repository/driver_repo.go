package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// DriverRepo reads the seeded Drivers. There is no write method, and that is ADR-0003 rather than an
// omission: ingest resolves against this table and fails on a name it does not hold, because
// inserting one splits a person's results across two ids without saying so.
type DriverRepo struct{}

func NewDriverRepo() *DriverRepo { return &DriverRepo{} }

// NamesToIDs loads the whole table as full_name -> id, the same shape and for the same reason
// CircuitRepo.KeysToIDs does: 29 rows read once at the top of a run, and every resolution after that
// is a map hit made before any transaction opens.
//
// full_name is the key because it is the only thing OpenF1 carries that identifies a person at all;
// the column is unique, so a name resolves to one Driver or to none.
func (r *DriverRepo) NamesToIDs(ctx context.Context, db sqlx.ExtContext) (map[string]uuid.UUID, error) {
	const q = `SELECT full_name, id FROM f1.drivers`

	rows, err := db.QueryxContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("driver_repo.NamesToIDs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make(map[string]uuid.UUID)
	for rows.Next() {
		var (
			fullName string
			id       uuid.UUID
		)
		if err := rows.Scan(&fullName, &id); err != nil {
			return nil, fmt.Errorf("driver_repo.NamesToIDs scan: %w", err)
		}
		ids[fullName] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("driver_repo.NamesToIDs: %w", err)
	}
	return ids, nil
}
