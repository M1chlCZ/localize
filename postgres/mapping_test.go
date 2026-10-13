package postgres

import (
	"testing"

	"github.com/m1chlcz/localize/workbook"
)

func TestLocaleFromRowMapsFallback(t *testing.T) {
	t.Parallel()
	fallback := "cs-CZ"
	locale := localeFromRow("en-GB", "English", "en", &fallback, true, false)
	if locale.FallbackCode != "cs-CZ" || !locale.Enabled || locale.Default {
		t.Fatalf("locale = %#v", locale)
	}
	withoutFallback := localeFromRow("cs-CZ", "Čeština", "", nil, true, true)
	if withoutFallback.FallbackCode != "" || !withoutFallback.Default {
		t.Fatalf("locale = %#v", withoutFallback)
	}
}

func TestTranslationFromStoredMapsStatus(t *testing.T) {
	t.Parallel()
	key := databaseKey{Name: "ui.test.title", Context: "Test title"}
	stored := translationFromStored(key, "en-GB", "Title", true)
	if stored.Status != workbook.StatusTranslated || stored.Translation != "Title" ||
		stored.Key != key.Name || stored.Locale != "en-GB" || stored.Context != key.Context {
		t.Fatalf("stored translation = %#v", stored)
	}
	missing := translationFromStored(key, "de-DE", "", false)
	if missing.Status != workbook.StatusMissing || missing.Translation != "" {
		t.Fatalf("missing translation = %#v", missing)
	}
}

func TestSameKeysComparesWorkbookMetadata(t *testing.T) {
	t.Parallel()
	authoritative := []databaseKey{
		{Name: "catalog.product.1.name", TargetKind: targetProduct, TargetID: "1", TargetField: targetFieldName, Context: "Product one name", Required: true},
		{Name: "ui.test.title", TargetKind: targetUI, TargetField: targetFieldTranslation, Context: "Test title", Required: false},
	}
	incoming := []workbook.Key{
		{Name: "ui.test.title", Context: "Test title", Required: false},
		{Name: "catalog.product.1.name", Context: "Product one name", Required: true},
	}
	if !sameKeys(authoritative, incoming) {
		t.Fatal("sameKeys rejected matching metadata in a different order")
	}
	for _, mutate := range []func([]workbook.Key) []workbook.Key{
		func(keys []workbook.Key) []workbook.Key {
			keys[0].Context = "Other"
			return keys
		},
		func(keys []workbook.Key) []workbook.Key {
			keys[0].Required = true
			return keys
		},
		func(keys []workbook.Key) []workbook.Key {
			return keys[:1]
		},
		func(keys []workbook.Key) []workbook.Key {
			keys[1].Name = "ui.test.other"
			return keys
		},
	} {
		changed := []workbook.Key{
			{Name: "ui.test.title", Context: "Test title", Required: false},
			{Name: "catalog.product.1.name", Context: "Product one name", Required: true},
		}
		if sameKeys(authoritative, mutate(changed)) {
			t.Fatal("sameKeys accepted changed metadata")
		}
	}
}

func TestLocalesDifferComparesEveryField(t *testing.T) {
	t.Parallel()
	current := []workbook.Locale{
		{Code: "cs-CZ", NativeName: "Čeština", Enabled: true, Default: true},
		{Code: "en-GB", NativeName: "English", RoutePrefix: "en", FallbackCode: "cs-CZ", Enabled: true},
	}
	reordered := []workbook.Locale{current[1], current[0]}
	if localesDiffer(current, reordered) {
		t.Fatal("localesDiffer rejected the same locales in a different order")
	}
	changed := []workbook.Locale{current[0], current[1]}
	changed[1].NativeName = "English (UK)"
	if !localesDiffer(current, changed) {
		t.Fatal("localesDiffer accepted a changed native name")
	}
	extra := append([]workbook.Locale{}, current...)
	extra = append(extra, workbook.Locale{Code: "de-DE", NativeName: "Deutsch", RoutePrefix: "de"})
	if !localesDiffer(current, extra) {
		t.Fatal("localesDiffer accepted an extra locale")
	}
}

func TestTranslationsDifferMirrorsApplySemantics(t *testing.T) {
	t.Parallel()
	actual := map[string]string{
		translationIdentity("ui.test.title", "en-GB"): "Title",
		translationIdentity("ui.test.old", "en-GB"):   "Old",
	}
	if translationsDiffer([]workbook.Translation{
		{Key: "ui.test.title", Locale: "en-GB", Translation: "Title", Status: workbook.StatusTranslated},
		{Key: "ui.test.old", Locale: "en-GB", Status: workbook.StatusMissing},
	}, actual) {
		t.Fatal("translationsDiffer reported a change for stored values")
	}
	if !translationsDiffer([]workbook.Translation{
		{Key: "ui.test.title", Locale: "en-GB", Translation: "Changed", Status: workbook.StatusTranslated},
	}, actual) {
		t.Fatal("translationsDiffer missed a changed value")
	}
	if !translationsDiffer([]workbook.Translation{
		{Key: "ui.test.new", Locale: "en-GB", Translation: "New", Status: workbook.StatusTranslated},
	}, actual) {
		t.Fatal("translationsDiffer missed a new value")
	}
	if !translationsDiffer([]workbook.Translation{
		{Key: "ui.test.old", Locale: "en-GB", Status: workbook.StatusDelete},
	}, actual) {
		t.Fatal("translationsDiffer missed a deletion")
	}
	if translationsDiffer([]workbook.Translation{
		{Key: "ui.test.absent", Locale: "en-GB", Status: workbook.StatusDelete},
	}, actual) {
		t.Fatal("translationsDiffer reported a change for an absent deletion")
	}
}
