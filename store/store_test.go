package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/m1chlcz/localize/workbook"
)

func testState() State {
	return State{
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
	incoming := Snapshot(state)
	for index := range incoming.Translations {
		if incoming.Translations[index].Locale == "de-DE" && incoming.Translations[index].Key == "page.title" {
			incoming.Translations[index].Translation = "Guten Tag"
		}
	}
	next, changes, err := Merge(state, incoming)
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
			incoming := Snapshot(state)
			test.mutate(&incoming)
			if _, _, err := Merge(state, incoming); err == nil {
				t.Fatalf("merge accepted %s", test.name)
			}
		})
	}
}

func TestMergeDeletesTranslations(t *testing.T) {
	t.Parallel()
	state := testState()
	incoming := Snapshot(state)
	for index := range incoming.Translations {
		if incoming.Translations[index].Locale == "de-DE" && incoming.Translations[index].Key == "page.subtitle" {
			incoming.Translations[index].Translation = ""
			incoming.Translations[index].Status = workbook.StatusDelete
		}
	}
	next, changes, err := Merge(state, incoming)
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
	if err := (JSONStore{Path: path}).save(testState()); err != nil {
		t.Fatalf("save store: %v", err)
	}
	ctx := context.Background()
	snapshot, err := (JSONStore{Path: path}).Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.Revision != 4 || len(snapshot.Translations) != 4 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	for index := range snapshot.Translations {
		if snapshot.Translations[index].Locale == "de-DE" && snapshot.Translations[index].Key == "page.title" {
			snapshot.Translations[index].Translation = "Servus"
		}
	}
	result, err := (JSONStore{Path: path}).Apply(ctx, snapshot, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.Applied || result.Changes != 1 || result.Revision != 5 {
		t.Fatalf("result = %#v", result)
	}
	updated, err := (JSONStore{Path: path}).Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot again: %v", err)
	}
	if updated.Revision != 5 {
		t.Fatalf("updated revision = %d", updated.Revision)
	}
	var german string
	for _, item := range updated.Translations {
		if item.Locale == "de-DE" && item.Key == "page.title" {
			german = item.Translation
		}
	}
	if german != "Servus" {
		t.Fatalf("updated snapshot = %#v", updated)
	}
	if _, err := (JSONStore{Path: path}).Apply(ctx, snapshot, false); !errors.Is(err, ErrStale) {
		t.Fatalf("stale apply error = %v", err)
	}
	dryRun, err := (JSONStore{Path: path}).Apply(ctx, updated, true)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if dryRun.Applied || dryRun.Changes != 0 {
		t.Fatalf("dry-run result = %#v", dryRun)
	}
}
