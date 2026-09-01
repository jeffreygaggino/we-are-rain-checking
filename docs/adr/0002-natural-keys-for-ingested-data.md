# Natural keys for ingested data

`STRUCTURE-go-gin-backend.md` calls for UUID primary keys on every table, a `created_by` column, and soft delete as the default. None of the three fits data this service does not author: OpenF1 already supplies stable identifiers (`meeting_key`, `session_key`), no user creates these rows, and rows are re-ingested rather than deleted.

Ingested tables therefore use the upstream identifiers as primary keys, carry `created_at` and `updated_at`, and carry neither `created_by` nor `deleted_at`.

## Considered options

A UUID primary key alongside a unique constraint on the natural key was the obvious compromise, and we rejected it. Resumable ingest needs `ON CONFLICT (natural key) DO UPDATE`, so the unique constraint has to exist regardless — at which point the UUID carries no information, is never the thing anything joins on, and exists only to satisfy a convention written for user-authored data.

## Consequences

This applies to ingested tables only. Tables this repo authors and seeds — circuits, drivers — have no upstream key to inherit and use a repo-owned surrogate id instead. See ADR-0003.
