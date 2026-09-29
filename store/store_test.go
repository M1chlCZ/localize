package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/m1chlcz/localize/store"
	"github.com/m1chlcz/localize/workbook"
)

func testState() store.State {
	return store.State{
		Version:  workbook.Version,
		Revision: 4,
		Locales: []workbook.Locale{
			{Code: "en-GB", NativeName: "English", Enabled: true, Default: true},
			{Code: "de-DE", NativeName: "Deutsch", RoutePrefix: "de", FallbackCode: "en-GB", Enabled: true},
		},
		Keys: []workbook.Key{
			{Name: "page.subtitle", Context: "Page subtitle", Required: false},
			{Name: "page.title", Context: "Page title", Required: true},
		},
		Translations: map[string]map[string]string{
			"en-GB": {"page.title": "Welcome", "page.subtitle": "An example"},
			"de-DE": {"page.title": "Willkommen", "page.subtitle": "Ein Beispiel"},
		},
	}
}

func TestMergeUpdatesTranslationsAndBumpsRevision(t *testing.T) {
	t.Parallel()
	state := testState()
	incoming := store.Snapshot(state)
	setTranslation(&incoming, "de-DE", "page.title", "Guten Tag")
	next, changes, err := store.Merge(state, incoming)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if changes != 1 || next.Revision != state.Revision+1 {
		t.Fatalf("changes = %d, revision = %d", changes, next.Revision)
	}
	if next.Translations["de-DE"]["page.title"] != "Guten Tag" {
		t.Fatalf("translation = %q", next.Translations["de-DE"]["page.title"])
	}
	if state.Translations["de-DE"]["page.title"] != "Willkommen" {
		t.Fatal("merge mutated the input state")
	}
}

func TestMergeRejectsUnsafeWorkbooks(t *testing.T) {
	t.Parallel()
	state := testState()
	for _, test := range []struct {
		name, want string
		mutate     func(*workbook.Workbook)
	}{
		{name: "stale revision", want: "stale", mutate: func(w *workbook.Workbook) { w.Revision++ }},
		{name: "changed key context", want: "keys do not match", mutate: func(w *workbook.Workbook) {
			w.Keys[0].Context = "Other"
			for index := range w.Translations {
				w.Translations[index].Context = "Other"
			}
		}},
		{name: "removed locale", want: "missing from the workbook", mutate: func(w *workbook.Workbook) {
			w.Locales = w.Locales[:1]
			kept := w.Translations[:0]
			for _, item := range w.Translations {
				if item.Locale == "en-GB" {
					kept = append(kept, item)
				}
			}
			w.Translations = kept
		}},
		{name: "changed default", want: "default locale", mutate: func(w *workbook.Workbook) {
			w.Locales[0].Default = false
			w.Locales[0].RoutePrefix = "en"
			w.Locales[0].FallbackCode = "de-DE"
			w.Locales[1].Default = true
			w.Locales[1].RoutePrefix = ""
			w.Locales[1].FallbackCode = ""
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			incoming := store.Snapshot(state)
			test.mutate(&incoming)
			if _, _, err := store.Merge(state, incoming); err == nil {
				t.Fatalf("merge accepted %s", test.name)
			}
		})
	}
}

func TestMergeDeletesTranslations(t *testing.T) {
	t.Parallel()
	state := testState()
	incoming := store.Snapshot(state)
	for index := range incoming.Translations {
		if incoming.Translations[index].Locale == "de-DE" && incoming.Translations[index].Key == "page.subtitle" {
			incoming.Translations[index].Translation = ""
			incoming.Translations[index].Status = workbook.StatusDelete
		}
	}
	next, changes, err := store.Merge(state, incoming)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if changes != 1 {
		t.Fatalf("changes = %d", changes)
	}
	if next.Translations["de-DE"]["page.subtitle"] != "" {
		t.Fatalf("de-DE subtitle = %q", next.Translations["de-DE"]["page.subtitle"])
	}
}

func TestJSONStoreRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "store.json")
	writeStore(t, path, testState())
	ctx := context.Background()
	snapshot := snapshotStore(t, path)
	if snapshot.Revision != 4 || len(snapshot.Translations) != 4 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	setTranslation(&snapshot, "de-DE", "page.title", "Servus")
	result, err := (store.JSONStore{Path: path}).Apply(ctx, snapshot, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.Applied || result.Changes != 1 || result.Revision != 5 {
		t.Fatalf("result = %#v", result)
	}
	updated := snapshotStore(t, path)
	if updated.Revision != 5 {
		t.Fatalf("updated revision = %d", updated.Revision)
	}
	if german := translationValue(updated, "de-DE", "page.title"); german != "Servus" {
		t.Fatalf("updated snapshot = %#v", updated)
	}
	if _, applyErr := (store.JSONStore{Path: path}).Apply(ctx, snapshot, false); !errors.Is(applyErr, store.ErrStale) {
		t.Fatalf("stale apply error = %v", applyErr)
	}
	dryRun, err := (store.JSONStore{Path: path}).Apply(ctx, updated, true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if dryRun.Applied || dryRun.Changes != 0 {
		t.Fatalf("dry-run result = %#v", dryRun)
	}
}

func writeStore(t *testing.T, path string, state store.State) {
	t.Helper()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal store: %v", err)
	}
	data = append(data, '\n')
	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		t.Fatalf("write store: %v", writeErr)
	}
}

func snapshotStore(t *testing.T, path string) workbook.Workbook {
	t.Helper()
	snapshot, err := (store.JSONStore{Path: path}).Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snapshot
}

func setTranslation(item *workbook.Workbook, locale, key, value string) {
	for index := range item.Translations {
		if item.Translations[index].Locale == locale && item.Translations[index].Key == key {
			item.Translations[index].Translation = value
		}
	}
}

func translationValue(item workbook.Workbook, locale, key string) string {
	for _, translation := range item.Translations {
		if translation.Locale == locale && translation.Key == key {
			return translation.Translation
		}
	}
	return ""
}
