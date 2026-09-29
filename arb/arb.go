package arb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/m1chlcz/localize/workbook"
)

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

type Artifact struct {
	Path    string
	Content []byte
}

// Generate renders one ARB file for each enabled locale, plus a TypeScript key
// union and a TypeScript defaults bundle for the default locale.
func Generate(item workbook.Workbook) ([]Artifact, error) {
	if err := workbook.Validate(item); err != nil {
		return nil, fmt.Errorf("validate localization workbook: %w", err)
	}
	locales := enabledLocales(item.Locales)
	keys := sortedKeys(item.Keys)
	if len(keys) == 0 {
		return nil, errors.New("workbook contains no keys")
	}
	translations := translationIndex(item.Translations)
	localeIndex := make(map[string]workbook.Locale, len(item.Locales))
	defaultLocale := ""
	for _, locale := range item.Locales {
		localeIndex[locale.Code] = locale
		if locale.Default {
			defaultLocale = locale.Code
		}
	}

	defaultMessages, err := messagesForLocale(defaultLocale, keys, translations, localeIndex)
	if err != nil {
		return nil, err
	}
	defaultPlaceholders := make(map[string][]string, len(keys))
	for _, key := range keys {
		defaultPlaceholders[key.Name] = placeholders(defaultMessages[key.Name])
	}

	artifacts := make([]Artifact, 0, len(locales))
	for _, locale := range locales {
		messages, messageErr := messagesForLocale(locale.Code, keys, translations, localeIndex)
		if messageErr != nil {
			return nil, messageErr
		}
		if placeholderErr := validatePlaceholders(
			locale.Code,
			keys,
			messages,
			defaultPlaceholders,
		); placeholderErr != nil {
			return nil, placeholderErr
		}
		content, marshalErr := marshalARB(locale.Code, keys, messages, defaultPlaceholders)
		if marshalErr != nil {
			return nil, marshalErr
		}
		artifacts = append(artifacts, Artifact{Path: locale.Code + ".arb", Content: content})
	}
	artifacts = append(artifacts,
		Artifact{Path: "defaults.ts", Content: renderDefaults(defaultLocale, keys, defaultMessages)},
		Artifact{Path: "keys.ts", Content: renderKeys(keys)},
	)
	slices.SortFunc(artifacts, func(left, right Artifact) int { return strings.Compare(left.Path, right.Path) })
	return artifacts, nil
}

func enabledLocales(items []workbook.Locale) []workbook.Locale {
	result := make([]workbook.Locale, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			result = append(result, item)
		}
	}
	slices.SortFunc(result, func(left, right workbook.Locale) int { return strings.Compare(left.Code, right.Code) })
	return result
}

func sortedKeys(items []workbook.Key) []workbook.Key {
	result := slices.Clone(items)
	slices.SortFunc(result, func(left, right workbook.Key) int { return strings.Compare(left.Name, right.Name) })
	return result
}

func translationIndex(items []workbook.Translation) map[string]workbook.Translation {
	result := make(map[string]workbook.Translation, len(items))
	for _, item := range items {
		result[item.Key+"\x00"+item.Locale] = item
	}
	return result
}

func messagesForLocale(
	localeCode string,
	keys []workbook.Key,
	translations map[string]workbook.Translation,
	locales map[string]workbook.Locale,
) (map[string]string, error) {
	messages := make(map[string]string, len(keys))
	for _, key := range keys {
		value, ok := resolveTranslation(key.Name, localeCode, translations, locales)
		if !ok {
			return nil, fmt.Errorf(
				"key %s has no translated value for locale %s or its fallback",
				key.Name,
				localeCode,
			)
		}
		messages[key.Name] = value
	}
	return messages, nil
}

func resolveTranslation(
	key string,
	localeCode string,
	translations map[string]workbook.Translation,
	locales map[string]workbook.Locale,
) (string, bool) {
	for code := localeCode; code != ""; code = locales[code].FallbackCode {
		translation := translations[key+"\x00"+code]
		if translation.Status == workbook.StatusTranslated {
			return translation.Translation, true
		}
	}
	return "", false
}

func placeholders(message string) []string {
	matches := placeholderPattern.FindAllStringSubmatch(message, -1)
	unique := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		unique[match[1]] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for name := range unique {
		result = append(result, name)
	}
	slices.Sort(result)
	return result
}

func validatePlaceholders(
	localeCode string,
	keys []workbook.Key,
	messages map[string]string,
	want map[string][]string,
) error {
	for _, key := range keys {
		got := placeholders(messages[key.Name])
		if !slices.Equal(got, want[key.Name]) {
			return fmt.Errorf(
				"key %s has placeholder mismatch in %s: got %v, want %v",
				key.Name,
				localeCode,
				got,
				want[key.Name],
			)
		}
	}
	return nil
}

func marshalARB(
	localeCode string,
	keys []workbook.Key,
	messages map[string]string,
	messagePlaceholders map[string][]string,
) ([]byte, error) {
	document := map[string]any{
		"@@locale": localeCode,
	}
	for _, key := range keys {
		document[key.Name] = messages[key.Name]
		metadata := map[string]any{"description": key.Context}
		if len(messagePlaceholders[key.Name]) > 0 {
			items := make(map[string]any, len(messagePlaceholders[key.Name]))
			for _, name := range messagePlaceholders[key.Name] {
				items[name] = map[string]string{"type": "String"}
			}
			metadata["placeholders"] = items
		}
		document["@"+key.Name] = metadata
	}
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s ARB: %w", localeCode, err)
	}
	return append(content, '\n'), nil
}

func renderKeys(keys []workbook.Key) []byte {
	var output bytes.Buffer
	output.WriteString("// Code generated by localize generate; DO NOT EDIT.\n\n")
	output.WriteString("export const translationKeys = [\n")
	for _, key := range keys {
		fmt.Fprintf(&output, "  %s,\n", quoted(key.Name))
	}
	output.WriteString("] as const;\n\n")
	output.WriteString("export type TranslationKey = (typeof translationKeys)[number];\n")
	return output.Bytes()
}

func renderDefaults(defaultLocale string, keys []workbook.Key, messages map[string]string) []byte {
	var output bytes.Buffer
	output.WriteString("// Code generated by localize generate; DO NOT EDIT.\n\n")
	output.WriteString("import type { TranslationKey } from \"./keys\";\n\n")
	fmt.Fprintf(&output, "export const defaultLocale = %s as const;\n\n", quoted(defaultLocale))
	output.WriteString("export const defaults: Readonly<Record<TranslationKey, string>> = {\n")
	for _, key := range keys {
		fmt.Fprintf(&output, "  %s: %s,\n", quoted(key.Name), quoted(messages[key.Name]))
	}
	output.WriteString("};\n")
	return output.Bytes()
}

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
