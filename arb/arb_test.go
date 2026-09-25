package arb_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/m1chlcz/localize/arb"
	"github.com/m1chlcz/localize/workbook"
)

func TestGenerateCreatesDeterministicArtifacts(t *testing.T) {
	t.Parallel()
	item := testWorkbook()
	first, err := arb.Generate(item)
	if err != nil {
		t.Fatalf("generate artifacts: %v", err)
	}
	second, err := arb.Generate(item)
	if err != nil {
		t.Fatalf("generate artifacts again: %v", err)
	}
	if len(first) != 4 || len(second) != 4 {
		t.Fatalf("artifact counts = %d and %d, want 4", len(first), len(second))
	}
	wantPaths := []string{"de-DE.arb", "defaults.ts", "en-GB.arb", "keys.ts"}
	for index, wantPath := range wantPaths {
		if first[index].Path != wantPath {
			t.Fatalf("artifact %d path = %q, want %q", index, first[index].Path, wantPath)
		}
		if string(first[index].Content) != string(second[index].Content) {
			t.Fatalf("artifact %s is not deterministic", wantPath)
		}
	}
	assertGermanARB(t, first[0].Content)
	assertTypeScriptArtifacts(t, first[1].Content, first[3].Content)
}

func TestGenerateRejectsPlaceholderMismatch(t *testing.T) {
	t.Parallel()
	item := testWorkbook()
	for index := range item.Translations {
		if item.Translations[index].Key == "cart.items" && item.Translations[index].Locale == "de-DE" {
			item.Translations[index].Translation = "Warenkorb hat {total} Artikel"
		}
	}
	_, err := arb.Generate(item)
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("Generate() error = %v, want placeholder mismatch", err)
	}
}

func assertGermanARB(t *testing.T, content []byte) {
	t.Helper()
	var german map[string]any
	if err := json.Unmarshal(content, &german); err != nil {
		t.Fatalf("decode German ARB: %v", err)
	}
	if german["@@locale"] != "de-DE" || german["cart.items"] != "Warenkorb hat {count} Artikel" {
		t.Fatalf("unexpected German ARB: %#v", german)
	}
	metadata, ok := german["@cart.items"].(map[string]any)
	if !ok || metadata["description"] != "Cart summary" {
		t.Fatalf("unexpected message metadata: %#v", german["@cart.items"])
	}
	placeholders, ok := metadata["placeholders"].(map[string]any)
	if !ok {
		t.Fatalf("missing placeholder metadata: %#v", metadata)
	}
	count, ok := placeholders["count"].(map[string]any)
	if !ok || count["type"] != "String" {
		t.Fatalf("unexpected count placeholder: %#v", placeholders["count"])
	}
}

func assertTypeScriptArtifacts(t *testing.T, defaultsContent, keysContent []byte) {
	t.Helper()
	defaults := string(defaultsContent)
	if !strings.Contains(defaults, `export const defaultLocale = "en-GB" as const;`) ||
		!strings.Contains(defaults, `"cart.items": "Cart has {count} items"`) {
		t.Fatalf("unexpected defaults bundle:\n%s", defaults)
	}
	keys := string(keysContent)
	if !strings.Contains(keys, `"cart.items"`) || !strings.Contains(keys, "TranslationKey") {
		t.Fatalf("unexpected keys bundle:\n%s", keys)
	}
}

func testWorkbook() workbook.Workbook {
	return workbook.Workbook{
		Version:  workbook.Version,
		Revision: 7,
		Locales: []workbook.Locale{
			{Code: "en-GB", NativeName: "English", Enabled: true, Default: true},
			{Code: "de-DE", NativeName: "Deutsch", RoutePrefix: "de", FallbackCode: "en-GB", Enabled: true},
		},
		Keys: []workbook.Key{
			{Name: "cart.items", Context: "Cart summary", Required: true},
			{Name: "page.title", Context: "Page title", Required: true},
		},
		Translations: []workbook.Translation{
			translation("cart.items", "en-GB", "Cart has {count} items", "Cart summary"),
			translation("cart.items", "de-DE", "Warenkorb hat {count} Artikel", "Cart summary"),
			translation("page.title", "en-GB", "Welcome", "Page title"),
			translation("page.title", "de-DE", "Willkommen", "Page title"),
		},
	}
}

func translation(key, locale, value, context string) workbook.Translation {
	return workbook.Translation{
		Key: key, Locale: locale, Translation: value, Context: context, Status: workbook.StatusTranslated,
	}
}
