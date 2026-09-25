package workbook

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWorkbookRoundTripPreservesLanguageColumns(t *testing.T) {
	t.Parallel()
	want := validWorkbook()
	want.ExportedAt = "2026-08-14T17:00:00Z"
	var encoded bytes.Buffer
	if err := Encode(&encoded, want); err != nil {
		t.Fatalf("encode workbook: %v", err)
	}
	got, err := Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("decode workbook: %v", err)
	}
	if got.Version != want.Version || got.Revision != want.Revision || got.ExportedAt != want.ExportedAt ||
		len(got.Locales) != 2 || len(got.Keys) != 1 || len(got.Translations) != 2 {
		t.Fatalf("decoded workbook = %#v", got)
	}
	if got.Locales[0].Code != "de-DE" || got.Locales[0].FallbackCode != "en-GB" ||
		got.Translations[0].Translation != "Willkommen" || got.Translations[1].Translation != "Welcome" {
		t.Fatalf("decoded rows = locales %#v translations %#v", got.Locales, got.Translations)
	}
}

func TestWorkbookWritesTranslationsSideBySide(t *testing.T) {
	t.Parallel()
	var encoded bytes.Buffer
	if err := Encode(&encoded, validWorkbook()); err != nil {
		t.Fatalf("encode workbook: %v", err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	rows, err := file.GetRows(translationsSheet)
	if err != nil {
		t.Fatalf("read translations: %v", err)
	}
	if got := rows[0]; !slices.Equal(got, []string{"key", "context", "de-DE", "en-GB"}) {
		t.Fatalf("translation headers = %#v", got)
	}
	if got := rows[1]; !slices.Equal(got, []string{"page.title", "Page title", "Willkommen", "Welcome"}) {
		t.Fatalf("translation row = %#v", got)
	}
}

func TestDecodeRejectsFormulaCells(t *testing.T) {
	t.Parallel()
	file := excelize.NewFile()
	t.Cleanup(func() { _ = file.Close() })
	if err := file.SetSheetName("Sheet1", translationsSheet); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	if err := file.SetCellFormula(translationsSheet, "A1", `HYPERLINK("https://example.invalid","key")`); err != nil {
		t.Fatalf("set formula: %v", err)
	}
	var encoded bytes.Buffer
	if err := file.Write(&encoded); err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	_, err := Decode(bytes.NewReader(encoded.Bytes()))
	if err == nil || !strings.Contains(err.Error(), "formula") {
		t.Fatalf("Decode() error = %v, want formula rejection", err)
	}
}
