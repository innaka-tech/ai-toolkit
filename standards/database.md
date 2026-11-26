# Databases and data

Data outlives code. Integrity lives in the database, every change is reversible, and backups count only when a restore has been tested.

## Schema
- Every table has a primary key; relationships have foreign keys; required columns are NOT NULL; business rules that can be constraints are (UNIQUE, CHECK).
- Names are consistent (snake_case, singular or plural chosen once); audit columns `created_at`, `updated_at` (UTC) and, where history matters, `created_by`/`updated_by`.
- Soft delete only where the business needs undo or audit; otherwise delete and keep an audit log. Financial and stock movements are append-only (corrections are new rows).
- Money in DECIMAL/NUMERIC or integer minor units; never FLOAT.

## Migrations
- Versioned files in the repository, applied by a tool (Laravel, EF Core, Flyway, Alembic, golang-migrate …), never by hand in production.
- Every migration has a tested down path or a written forward-fix plan.
- Zero-downtime changes use expand → migrate data → contract across releases: add the new column or table, write to both, backfill in batches, switch reads, then remove the old one in a later release.
- Large tables: no locking ALTERs during business hours; batch backfills; check the execution plan first.
- Back up (or snapshot) before any destructive migration and state the restore command in the task.

## Queries
- Parameterised always. Select the columns you need. Index for the real query patterns (check with EXPLAIN); every foreign key used in joins is indexed.
- Transactions around multi-statement changes that must succeed together; keep them short; choose the isolation level deliberately; handle deadlock retries.
- No N+1 queries; paginate with keyset where tables are large.

## Access and protection
- Application accounts have least privilege (no DDL or superuser at runtime); separate accounts per service; no shared personal accounts.
- Personal and sensitive data classified, encrypted where required, masked in non-production copies.

## Backup and recovery
- 3-2-1: three copies, two media, one off-site/immutable. Point-in-time recovery for transactional systems.
- RPO and RTO written down per system; a restore test at least quarterly with the measured time recorded.
