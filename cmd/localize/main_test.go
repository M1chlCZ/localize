package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/m1chlcz/localize/workbook"
)

func TestExecuteValidatesWorkbook(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "translations.xlsx")
	writeTestWorkbook(t, path)
	var output bytes.Buffer
	if err := execute(context.Background(), []string{"validate", "--input", path}, &output); err != nil {
		t.Fatalf("validate command: %v", err)
	}
	if !strings.Contains(output.String(), "valid") {
		t.Fatalf("validate output = %q", output.String())
	}
}

func TestExecuteGeneratesArtifacts(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	workbookPath := filepath.Join(directory, "translations.xlsx")
	writeTestWorkbook(t, workbookPath)
	outputDirectory := filepath.Join(directory, "generated")
	var output bytes.Buffer
	if err := execute(context.Background(), []string{
		"generate", "--input", workbookPath, "--output-dir", outputDirectory,
	}, &output); err != nil {
		t.Fatalf("generate command: %v", err)
	}
	for _, name := range []string{"de-DE.arb", "defaults.ts", "en-GB.arb", "keys.ts"} {
		content, err := os.ReadFile(filepath.Join(outputDirectory, name))
		if err != nil || len(content) == 0 {
			t.Fatalf("read generated %s: %v", name, err)
		}
	}
	if !strings.Contains(output.String(), "generated 4 localization artifacts") {
		t.Fatalf("generate output = %q", output.String())
	}
}

func TestExecuteRejectsAmbiguousImportMode(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"import", "--store", "store.json", "--input", "translations.xlsx"},
		{"import", "--store", "store.json", "--input", "translations.xlsx", "--dry-run", "--apply"},
	} {
		err := execute(context.Background(), args, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("execute(%v) error = %v", args, err)
		}
	}
}

func TestExamplesRoundTrip(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "examples")
	workbookPath := filepath.Join(root, "translations.xlsx")
	storePath := filepath.Join(t.TempDir(), "store.json")
	source, err := os.ReadFile(filepath.Join(root, "store.json"))
	if err != nil {
		t.Fatalf("read example store: %v", err)
	}
	if err := os.WriteFile(storePath, source, 0o600); err != nil {
		t.Fatalf("copy example store: %v", err)
	}

	var validateOutput bytes.Buffer
	if err := execute(context.Background(), []string{"validate", "--input", workbookPath}, &validateOutput); err != nil {
		t.Fatalf("validate example: %v", err)
	}

	var importOutput bytes.Buffer
	if err := execute(context.Background(), []string{
		"import", "--store", storePath, "--input", workbookPath, "--dry-run",
	}, &importOutput); err != nil {
		t.Fatalf("dry-run example: %v", err)
	}
	if !strings.Contains(importOutput.String(), "dry-run 0 changes") {
		t.Fatalf("dry-run output = %q", importOutput.String())
	}

	generated := t.TempDir()
	var generateOutput bytes.Buffer
	if err := execute(context.Background(), []string{
		"generate", "--input", workbookPath, "--output-dir", generated,
	}, &generateOutput); err != nil {
		t.Fatalf("generate example: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "arb"))
	if err != nil {
		t.Fatalf("read example artifacts: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("example artifact count = %d, want 4", len(entries))
	}
	for _, entry := range entries {
		want, err := os.ReadFile(filepath.Join(root, "arb", entry.Name()))
		if err != nil {
			t.Fatalf("read committed %s: %v", entry.Name(), err)
		}
		got, err := os.ReadFile(filepath.Join(generated, entry.Name()))
		if err != nil {
			t.Fatalf("read generated %s: %v", entry.Name(), err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s is stale; run localize generate again", entry.Name())
		}
	}
}

func writeTestWorkbook(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create workbook: %v", err)
	}
	if err := workbook.Encode(file, commandTestWorkbook()); err != nil {
		_ = file.Close()
		t.Fatalf("encode workbook: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close workbook: %v", err)
	}
}

func commandTestWorkbook() workbook.Workbook {
	return workbook.Workbook{
		Version: workbook.Version, Revision: 3, ExportedAt: "2026-08-14T17:00:00Z",
		Locales: []workbook.Locale{
			{Code: "en-GB", NativeName: "English", Enabled: true, Default: true},
			{Code: "de-DE", NativeName: "Deutsch", RoutePrefix: "de", FallbackCode: "en-GB", Enabled: true},
		},
		Keys: []workbook.Key{
			{Name: "page.title", Context: "Page title", Required: true},
		},
		Translations: []workbook.Translation{
			{Key: "page.title", Locale: "en-GB", Translation: "Welcome", Context: "Page title", Status: workbook.StatusTranslated},
			{Key: "page.title", Locale: "de-DE", Translation: "Willkommen", Context: "Page title", Status: workbook.StatusTranslated},
		},
	}
}
