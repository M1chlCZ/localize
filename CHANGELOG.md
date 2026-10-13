# Changelog

All notable changes to this package are documented in this file. The format
follows Keep a Changelog, and the project uses semantic versioning.

## [0.2.0] - 2026-10-13

### Added

- `postgres` adapter for the shop localization schema: `Open`, `Export`,
  `Apply`, and a `Store` that implements the `store.Store` interface. It reads
  all locales and keys, including product variant translations, and applies a
  workbook in one serializable, revision-checked transaction. A stale
  revision wraps `store.ErrStale`; dry runs and failures roll back cleanly; an
  unchanged workbook is an idempotent no-op that keeps the revision.
- `localize export --database-url URL` and
  `localize import --database-url URL`; when the flag is absent, the
  file-based commands are unchanged.
- Integration tests for the adapter, gated by
  `LOCALIZE_TEST_DATABASE_URL`.
