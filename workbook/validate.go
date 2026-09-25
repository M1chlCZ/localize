package workbook

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const maxLocales = 64

var (
	localeCodePattern  = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Z][a-z]{3})?-(?:[A-Z]{2}|[0-9]{3})$`)
	routePrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,15}$`)
	keyPattern         = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
)

// Validate checks the workbook version, the locales, the keys, and the
// translation coverage. It returns the first error that it finds.
func Validate(item Workbook) error {
	if item.Version != Version {
		return fmt.Errorf("unsupported workbook version %q", item.Version)
	}
	if item.Revision < 1 {
		return errors.New("workbook revision must be positive")
	}
	locales, err := validateLocales(item.Locales)
	if err != nil {
		return err
	}
	keys, err := validateKeys(item.Keys)
	if err != nil {
		return err
	}
	return validateTranslations(item.Translations, locales, keys)
}

func validateLocales(items []Locale) (map[string]Locale, error) {
	if len(items) == 0 {
		return nil, errors.New("workbook contains no languages")
	}
	if len(items) > maxLocales {
		return nil, fmt.Errorf("workbook contains more than %d languages", maxLocales)
	}
	locales := make(map[string]Locale, len(items))
	prefixes := make(map[string]string, len(items))
	defaultCount := 0
	for _, item := range items {
		if err := addValidatedLocale(locales, prefixes, item, &defaultCount); err != nil {
			return nil, err
		}
	}
	if defaultCount != 1 {
		return nil, fmt.Errorf("workbook must contain exactly one default locale, got %d", defaultCount)
	}
	if err := validateLocaleFallbacks(locales); err != nil {
		return nil, err
	}
	return locales, nil
}

func addValidatedLocale(
	locales map[string]Locale,
	prefixes map[string]string,
	item Locale,
	defaultCount *int,
) error {
	if !localeCodePattern.MatchString(item.Code) {
		return fmt.Errorf("locale %q is not a canonical BCP-47 language-region code", item.Code)
	}
	if item.NativeName == "" || strings.TrimSpace(item.NativeName) != item.NativeName {
		return fmt.Errorf("locale %s has an invalid native name", item.Code)
	}
	if _, exists := locales[item.Code]; exists {
		return fmt.Errorf("duplicate locale %s", item.Code)
	}
	if previous, exists := prefixes[item.RoutePrefix]; exists {
		return fmt.Errorf("route prefix %q is shared by %s and %s", item.RoutePrefix, previous, item.Code)
	}
	if item.Default {
		*defaultCount++
		if !item.Enabled || item.RoutePrefix != "" {
			return fmt.Errorf("default locale %s must be enabled with an empty route prefix", item.Code)
		}
	} else {
		expectedPrefix, _, _ := strings.Cut(item.Code, "-")
		if !routePrefixPattern.MatchString(item.RoutePrefix) || item.RoutePrefix != expectedPrefix {
			return fmt.Errorf("locale %s route prefix must be %q", item.Code, expectedPrefix)
		}
		if item.FallbackCode == "" {
			return fmt.Errorf("locale %s must have a fallback", item.Code)
		}
	}
	locales[item.Code] = item
	prefixes[item.RoutePrefix] = item.Code
	return nil
}

func validateLocaleFallbacks(locales map[string]Locale) error {
	for code, item := range locales {
		if item.FallbackCode != "" {
			if _, exists := locales[item.FallbackCode]; !exists {
				return fmt.Errorf("locale %s has unknown fallback %s", code, item.FallbackCode)
			}
		}
		visited := map[string]bool{code: true}
		for fallback := item.FallbackCode; fallback != ""; fallback = locales[fallback].FallbackCode {
			if visited[fallback] {
				return fmt.Errorf("locale %s participates in a fallback cycle", code)
			}
			visited[fallback] = true
		}
		if item.Default && item.FallbackCode != "" {
			return fmt.Errorf("default locale %s cannot have a fallback", item.Code)
		}
	}
	return nil
}

func validateKeys(items []Key) (map[string]Key, error) {
	if len(items) == 0 {
		return nil, errors.New("workbook contains no translation keys")
	}
	keys := make(map[string]Key, len(items))
	for _, item := range items {
		if !keyPattern.MatchString(item.Name) {
			return nil, fmt.Errorf("translation key %q is invalid", item.Name)
		}
		if _, exists := keys[item.Name]; exists {
			return nil, fmt.Errorf("duplicate translation key %s", item.Name)
		}
		if strings.TrimSpace(item.Context) != item.Context {
			return nil, fmt.Errorf("translation key %s has invalid context", item.Name)
		}
		keys[item.Name] = item
	}
	return keys, nil
}

func validateTranslations(items []Translation, locales map[string]Locale, keys map[string]Key) error {
	seen := make(map[string]Translation, len(items))
	for _, item := range items {
		identity, err := validateTranslationRow(item, locales, keys, seen)
		if err != nil {
			return err
		}
		seen[identity] = item
	}
	return validateTranslationCoverage(seen, locales, keys)
}

func validateTranslationRow(
	item Translation,
	locales map[string]Locale,
	keys map[string]Key,
	seen map[string]Translation,
) (string, error) {
	key, exists := keys[item.Key]
	if !exists {
		return "", fmt.Errorf("unknown translation key %s", item.Key)
	}
	if _, exists = locales[item.Locale]; !exists {
		return "", fmt.Errorf("translation %s has unknown locale %s", item.Key, item.Locale)
	}
	identity := item.Key + "\x00" + item.Locale
	if _, exists = seen[identity]; exists {
		return "", fmt.Errorf("duplicate translation for %s/%s", item.Key, item.Locale)
	}
	if item.Context != key.Context {
		return "", fmt.Errorf("translation %s/%s changed its context", item.Key, item.Locale)
	}
	if strings.TrimSpace(item.Translation) != item.Translation {
		return "", fmt.Errorf("translation %s/%s has surrounding whitespace", item.Key, item.Locale)
	}
	if err := validateTranslationStatus(item); err != nil {
		return "", err
	}
	return identity, nil
}

func validateTranslationStatus(item Translation) error {
	switch item.Status {
	case StatusTranslated:
		if item.Translation == "" {
			return fmt.Errorf("translated row %s/%s is empty", item.Key, item.Locale)
		}
		if item.Translation == deleteMarker {
			return fmt.Errorf("translation %s/%s uses the reserved delete marker", item.Key, item.Locale)
		}
	case StatusMissing, StatusDelete:
		if item.Translation != "" {
			return fmt.Errorf("%s row %s/%s must be empty", item.Status, item.Key, item.Locale)
		}
	default:
		return fmt.Errorf("translation %s/%s has invalid status %q", item.Key, item.Locale, item.Status)
	}
	return nil
}

func validateTranslationCoverage(
	seen map[string]Translation,
	locales map[string]Locale,
	keys map[string]Key,
) error {
	for localeCode, locale := range locales {
		for keyName, key := range keys {
			translation, exists := seen[keyName+"\x00"+localeCode]
			if !exists {
				return fmt.Errorf("translation row %s/%s is missing", keyName, localeCode)
			}
			if locale.Enabled && key.Required && translation.Status != StatusTranslated {
				return fmt.Errorf("enabled locale %s is missing required key %s", localeCode, keyName)
			}
		}
	}
	return nil
}
