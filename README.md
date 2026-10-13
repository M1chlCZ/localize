# localize

Round-trip translations through an XLSX workbook. Keep a revision guard. Generate ARB files and TypeScript artifacts.

## How it works

1. The store keeps the locales, the keys, and the translations. It also keeps a revision number. The store is a JSON file or the shop PostgreSQL database.
2. `localize export` writes the store to an XLSX workbook. The workbook records the revision.
3. A translator edits the workbook.
4. `localize import --dry-run` checks the workbook against the store. `localize import --apply` writes the changes.
5. `localize generate` writes one ARB file for each enabled locale. It also writes a TypeScript key union and a defaults bundle.

An import fails when the workbook revision is stale. In that case, export the workbook again.

## Install

```
go install github.com/m1chlcz/localize/cmd/localize@latest
```

Go 1.27 or later is required.

## Quick start

The directory `examples` contains a store and a generated workbook. Serve the directory over HTTP to open the example page:

```
python3 -m http.server 8000 --directory examples
```

Then open `http://localhost:8000`. The page reads the generated ARB files. Use the buttons to change the language.

Rebuild the example files with these commands:

```
localize export --store examples/store.json --output examples/translations.xlsx --force
localize validate --input examples/translations.xlsx
localize generate --input examples/translations.xlsx --output-dir examples/arb
```

## Commands

| Command | Purpose |
| --- | --- |
| `export --store FILE.json --output FILE.xlsx [--force]` | Write the store to a workbook. |
| `export --database-url URL --output FILE.xlsx [--force]` | Write the shop database to a workbook. |
| `validate --input FILE.xlsx` | Decode and validate a workbook. |
| `import --store FILE.json --input FILE.xlsx (--dry-run\|--apply)` | Apply a workbook to the store. |
| `import --database-url URL --input FILE.xlsx (--dry-run\|--apply)` | Apply a workbook to the shop database. |
| `generate --input FILE.xlsx --output-dir DIRECTORY` | Write the ARB and TypeScript artifacts. |

The `--database-url` flag is optional. Every file-based command keeps working
unchanged when the flag is absent.

## Workbook format

The workbook has four sheets.

| Sheet | Content |
| --- | --- |
| `Instructions` | Notes for translators. |
| `Languages` | One row for each locale. |
| `Translations` | One row for each key. One column for each locale. |
| `Metadata` | Generated. Do not edit. It holds the revision and the key list. |

Rules:

- Use canonical BCP-47 codes such as `en-GB` or `de-DE`.
- An empty cell means missing or unchanged.
- Enter `__DELETE__` to remove an existing translation.
- An enabled locale must hold every required translation.
- The import rejects formulas, unknown keys, duplicate rows, and stale revisions.

## Store

A store implements the `Store` interface. The package `store` holds a JSON file implementation.

```go
type Store interface {
	Snapshot(ctx context.Context) (workbook.Workbook, error)
	Apply(ctx context.Context, incoming workbook.Workbook, dryRun bool) (Result, error)
}
```

The JSON file holds the version, the revision, the locales, the keys, and a map of translations.

## PostgreSQL adapter

The package `postgres` implements the same `Store` interface against the shop
localization schema. It also exposes `Open`, `Export`, and `Apply`.

```go
pool, err := postgres.Open(ctx, databaseURL)
if err != nil {
    return err
}
defer pool.Close()
snapshot, err := postgres.Export(ctx, pool)
result, err := postgres.Apply(ctx, pool, snapshot, false)
```

The CLI uses the adapter through `--database-url`:

```
localize export --database-url 'postgres://user@host/shop' --output translations.xlsx --force
localize import --database-url 'postgres://user@host/shop' --input translations.xlsx --dry-run
localize import --database-url 'postgres://user@host/shop' --input translations.xlsx --apply
```

Semantics:

- `Export` reads every locale, key, and translation in a repeatable-read,
  read-only transaction.
- `Apply` validates the workbook, then locks the import with the
  `shop.localization.import` advisory lock and runs in a serializable
  transaction.
- A stale revision returns an error that wraps `store.ErrStale`.
- A dry run reports the changes and rolls back.
- A workbook that matches the stored state is a no-op: it reports zero
  changes, keeps the revision, and can be applied again.
- A workbook with changes advances the revision through the schema triggers,
  so the same workbook is stale after a successful apply.
- A failed apply rolls back every change of its transaction.
- The workbook format stores only key names, contexts, and required flags.
  The adapter reads the target kind, id, and field from the database and
  refuses a workbook whose keys, contexts, or required flags do not match.

### Required schema

The adapter never changes the schema. The database must already hold the shop
tables. The adapter reads and writes these tables and columns:

| Table | Columns |
| --- | --- |
| `shop.localization_state` | `singleton`, `revision` |
| `shop.locales` | `code`, `native_name`, `route_prefix`, `fallback_code`, `enabled`, `is_default`, `updated_at` |
| `shop.ui_translation_keys` | `key`, `context`, `required` |
| `shop.ui_translations` | `key`, `locale`, `translation` |
| `shop.categories` | `id`, `slug`, `active` |
| `shop.category_translations` | `category_id`, `locale`, `name` |
| `shop.subcategories` | `id`, `slug`, `active` |
| `shop.subcategory_translations` | `subcategory_id`, `locale`, `name` |
| `shop.products` | `id`, `slug`, `status` |
| `shop.product_translations` | `product_id`, `locale`, `name`, `description`, `safety_information`, `seo_title`, `seo_description` |
| `shop.product_variants` | `id`, `sku`, `name`, `updated_at` |
| `shop.product_variant_translations` | `variant_id`, `locale`, `name` |
| `shop.customization_fields` | `id`, `product_id`, `field_key`, `active` |
| `shop.customization_field_translations` | `field_id`, `locale`, `label` |
| `shop.customization_choices` | `id`, `field_id`, `value`, `active` |
| `shop.customization_choice_translations` | `choice_id`, `locale`, `label` |
| `shop.components` | `id`, `sku`, `active` |
| `shop.component_translations` | `component_id`, `locale`, `name` |

The revision triggers from the shop migrations must exist: every localized
table bumps `shop.localization_state.revision` with an
`AFTER ... FOR EACH STATEMENT` trigger. Product variant names use `cs-CZ` as
the source locale: an update writes `shop.product_variants.name`, every other
locale writes `shop.product_variant_translations.name`. The adapter refuses to
delete the source variant name.

## Generated artifacts

For each enabled locale, the tool writes `<locale>.arb`. An ARB file holds the locale code, the messages, and message metadata. The metadata holds the description and the placeholders.

The tool also writes `keys.ts` and `defaults.ts`. `keys.ts` exports the key union. `defaults.ts` exports the default locale and the default messages.

## Limits

| Item | Limit |
| --- | --- |
| Workbook size | 16 MiB |
| Workbook rows per sheet | 200000 |
| Cell length | 32000 characters |
| Store size | 16 MiB |
| Locales | 64 |

## Development

```
go test ./...
go vet ./...
```

The integration tests of the `postgres` package are skipped when
`LOCALIZE_TEST_DATABASE_URL` is unset. They drop and recreate the `shop`
schema in the target database, so use a dedicated database:

```
LOCALIZE_TEST_DATABASE_URL='postgres://user@127.0.0.1:5432/localize_test' go test -race ./postgres/
```

## License

MIT. See `LICENSE`.
