# localize

Round-trip translations through an XLSX workbook. Keep a revision guard. Generate ARB files and TypeScript artifacts.

## How it works

1. The store keeps the locales, the keys, and the translations. It also keeps a revision number.
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
| `validate --input FILE.xlsx` | Decode and validate a workbook. |
| `import --store FILE.json --input FILE.xlsx (--dry-run\|--apply)` | Apply a workbook to the store. |
| `generate --input FILE.xlsx --output-dir DIRECTORY` | Write the ARB and TypeScript artifacts. |

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

## License

MIT. See `LICENSE`.
