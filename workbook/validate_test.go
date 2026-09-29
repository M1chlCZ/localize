package workbook_test

import (
	"strings"
	"testing"

	"github.com/m1chlcz/localize/workbook"
)

func TestValidateAcceptsCompleteWorkbook(t *testing.T) {
	t.Parallel()
	if err := workbook.Validate(validWorkbook()); err != nil {
		t.Fatalf("validate workbook: %v", err)
	}
}

func TestValidateRejectsUnsafeWorkbooks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, want string
		mutate     func(*workbook.Workbook)
	}{
		{name: "legacy locale", want: "canonical BCP-47", mutate: func(w *workbook.Workbook) {
			w.Locales[1].Code = "de"
		}},
		{name: "duplicate route", want: "route prefix", mutate: func(w *workbook.Workbook) {
			w.Locales[1].RoutePrefix = ""
		}},
		{name: "fallback cycle", want: "fallback cycle", mutate: func(w *workbook.Workbook) {
			w.Locales[0].FallbackCode = "de-DE"
		}},
		{name: "duplicate translation", want: "duplicate translation", mutate: func(w *workbook.Workbook) {
			w.Translations = append(w.Translations, w.Translations[0])
		}},
		{name: "unknown key", want: "unknown translation key", mutate: func(w *workbook.Workbook) {
			w.Translations[0].Key = "page.unknown"
		}},
		{name: "enabled locale incomplete", want: "missing required", mutate: func(w *workbook.Workbook) {
			w.Translations[1].Translation = ""
			w.Translations[1].Status = workbook.StatusMissing
		}},
		{name: "implicit delete", want: "translated row", mutate: func(w *workbook.Workbook) {
			w.Translations[0].Translation = ""
		}},
		{name: "invalid key", want: "invalid", mutate: func(w *workbook.Workbook) {
			w.Keys[0].Name = "Page Title"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			item := validWorkbook()
			test.mutate(&item)
			err := workbook.Validate(item)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func validWorkbook() workbook.Workbook {
	return workbook.Workbook{
		Version:  workbook.Version,
		Revision: 7,
		Locales: []workbook.Locale{
			{Code: "en-GB", NativeName: "English", Enabled: true, Default: true},
			{Code: "de-DE", NativeName: "Deutsch", RoutePrefix: "de", FallbackCode: "en-GB", Enabled: true},
		},
		Keys: []workbook.Key{
			{Name: "page.title", Context: "Page title", Required: true},
		},
		Translations: []workbook.Translation{
			{
				Key: "page.title", Locale: "en-GB", Translation: "Welcome", Context: "Page title",
				Status: workbook.StatusTranslated,
			},
			{
				Key: "page.title", Locale: "de-DE", Translation: "Willkommen", Context: "Page title",
				Status: workbook.StatusTranslated,
			},
		},
	}
}
