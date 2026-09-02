package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// countsByYear runs a `SELECT year, COUNT(*) ... GROUP BY year` and returns the tally, never nil.
//
// It sits here rather than in either table's file because both Meetings and Sessions answer the same
// question — which seasons have landed — and "one file per table" is a rule about queries, not about
// the scan they share.
func countsByYear(ctx context.Context, db sqlx.ExtContext, q string) (map[int]int, error) {
	rows, err := db.QueryxContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[int]int)
	for rows.Next() {
		var year, count int
		if err := rows.Scan(&year, &count); err != nil {
			return nil, err
		}
		counts[year] = count
	}
	return counts, rows.Err()
}
