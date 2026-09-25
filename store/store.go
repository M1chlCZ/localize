package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/m1chlcz/localize/workbook"
)

var ErrStale = errors.New("workbook revision is stale")

type Result struct {
	Applied  bool
	Changes  int64
	Revision int64
}

type Store interface {
	Snapshot(ctx context.Context) (workbook.Workbook, error)
	Apply(ctx context.Context, incoming workbook.Workbook, dryRun bool) (Result, error)
}

type State struct {
	Version      string                       `json:"version"`
	Revision     int64                        `json:"revision"`
	Locales      []workbook.Locale            `json:"locales"`
	Keys         []workbook.Key               `json:"keys"`
	Translations map[string]map[string]string `json:"translations"`
}

// Snapshot turns stored state into a workbook with one row for every key and
// locale. A stored value becomes the translated status. An absent value
// becomes the missing status.
func Snapshot(state State) workbook.Workbook {
	locales := slices.Clone(state.Locales)
	keys := slices.Clone(state.Keys)
	item := workbook.Workbook{Version: state.Version, Revision: state.Revision, Locales: locales, Keys: keys}
	for _, key := range keys {
		for _, locale := range locales {
			value := state.Translations[locale.Code][key.Name]
			status := workbook.StatusMissing
			if value != "" {
				status = workbook.StatusTranslated
			}
			item.Translations = append(item.Translations, workbook.Translation{
				Key: key.Name, Locale: locale.Code, Translation: value, Context: key.Context, Status: status,
			})
		}
	}
	return item
}

// Merge applies a validated workbook to the state. It rejects a stale revision,
// a changed key set, a removed locale, and a changed default locale. It returns
// the next state and the number of changed records.
func Merge(state State, incoming workbook.Workbook) (State, int64, error) {
	if err := workbook.Validate(incoming); err != nil {
		return State{}, 0, err
	}
	if incoming.Revision != state.Revision {
		return State{}, 0, fmt.Errorf("%w: workbook=%d store=%d", ErrStale, incoming.Revision, state.Revision)
	}
	if !sameKeys(state.Keys, incoming.Keys) {
		return State{}, 0, errors.New("workbook keys do not match the store")
	}
	next := State{
		Version:      state.Version,
		Revision:     state.Revision,
		Locales:      slices.Clone(state.Locales),
		Keys:         slices.Clone(state.Keys),
		Translations: cloneTranslations(state.Translations),
	}
	changes, err := mergeLocales(&next, incoming.Locales)
	if err != nil {
		return State{}, 0, err
	}
	translationChanges := mergeTranslations(&next, incoming.Translations)
	changes += translationChanges
	if err := validateCoverage(next); err != nil {
		return State{}, 0, err
	}
	next.Revision++
	return next, changes, nil
}

func mergeLocales(next *State, incoming []workbook.Locale) (int64, error) {
	current := make(map[string]workbook.Locale, len(next.Locales))
	for _, locale := range next.Locales {
		current[locale.Code] = locale
	}
	seen := make(map[string]bool, len(incoming))
	var changes int64
	for _, locale := range incoming {
		seen[locale.Code] = true
		previous, exists := current[locale.Code]
		if exists && previous.Default != locale.Default {
			return 0, fmt.Errorf("changing the default locale is not supported")
		}
		if !exists || previous != locale {
			changes++
		}
	}
	for code := range current {
		if !seen[code] {
			return 0, fmt.Errorf("store locale %s is missing from the workbook", code)
		}
	}
	next.Locales = slices.Clone(incoming)
	slices.SortFunc(next.Locales, func(left, right workbook.Locale) int { return strings.Compare(left.Code, right.Code) })
	return changes, nil
}

func mergeTranslations(next *State, incoming []workbook.Translation) int64 {
	var changes int64
	for _, translation := range incoming {
		if translation.Status == workbook.StatusMissing {
			continue
		}
		values := next.Translations[translation.Locale]
		if translation.Status == workbook.StatusDelete {
			if values[translation.Key] == "" {
				continue
			}
			delete(values, translation.Key)
			if len(values) == 0 {
				delete(next.Translations, translation.Locale)
			}
			changes++
			continue
		}
		if values[translation.Key] == translation.Translation {
			continue
		}
		if values == nil {
			values = make(map[string]string)
			next.Translations[translation.Locale] = values
		}
		values[translation.Key] = translation.Translation
		changes++
	}
	return changes
}

func validateCoverage(state State) error {
	for _, locale := range state.Locales {
		if !locale.Enabled {
			continue
		}
		for _, key := range state.Keys {
			if key.Required && state.Translations[locale.Code][key.Name] == "" {
				return fmt.Errorf("enabled locale %s is missing required key %s", locale.Code, key.Name)
			}
		}
	}
	return nil
}

func sameKeys(left, right []workbook.Key) bool {
	left = slices.Clone(left)
	right = slices.Clone(right)
	slices.SortFunc(left, compareKey)
	slices.SortFunc(right, compareKey)
	return slices.Equal(left, right)
}

func compareKey(left, right workbook.Key) int { return strings.Compare(left.Name, right.Name) }

func cloneTranslations(source map[string]map[string]string) map[string]map[string]string {
	result := make(map[string]map[string]string, len(source))
	for locale, values := range source {
		copied := make(map[string]string, len(values))
		for key, value := range values {
			copied[key] = value
		}
		result[locale] = copied
	}
	return result
}
