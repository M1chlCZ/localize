package postgres

import (
	"slices"
	"strings"

	"github.com/m1chlcz/localize/workbook"
)

func localeFromRow(
	code, nativeName, routePrefix string,
	fallback *string,
	enabled, isDefault bool,
) workbook.Locale {
	locale := workbook.Locale{
		Code:        code,
		NativeName:  nativeName,
		RoutePrefix: routePrefix,
		Enabled:     enabled,
		Default:     isDefault,
	}
	if fallback != nil {
		locale.FallbackCode = *fallback
	}
	return locale
}

func translationFromStored(key databaseKey, locale, value string, stored bool) workbook.Translation {
	status := workbook.StatusMissing
	if stored {
		status = workbook.StatusTranslated
	}
	return workbook.Translation{
		Key:         key.Name,
		Locale:      locale,
		Translation: value,
		Context:     key.Context,
		Status:      status,
	}
}

func translationIdentity(key, locale string) string { return key + "\x00" + locale }

func sameKeys(authoritative []databaseKey, incoming []workbook.Key) bool {
	wanted := make([]workbook.Key, len(authoritative))
	for index, key := range authoritative {
		wanted[index] = key.workbookKey()
	}
	slices.SortFunc(wanted, compareKey)
	candidates := slices.Clone(incoming)
	slices.SortFunc(candidates, compareKey)
	return slices.Equal(wanted, candidates)
}

func compareKey(left, right workbook.Key) int { return strings.Compare(left.Name, right.Name) }

func localesDiffer(current, incoming []workbook.Locale) bool {
	left := slices.Clone(current)
	right := slices.Clone(incoming)
	slices.SortFunc(left, compareLocale)
	slices.SortFunc(right, compareLocale)
	return !slices.Equal(left, right)
}

func compareLocale(left, right workbook.Locale) int { return strings.Compare(left.Code, right.Code) }

func translationsDiffer(incoming []workbook.Translation, actual map[string]string) bool {
	for _, translation := range incoming {
		value, stored := actual[translationIdentity(translation.Key, translation.Locale)]
		switch translation.Status {
		case workbook.StatusMissing:
			continue
		case workbook.StatusDelete:
			if stored {
				return true
			}
		case workbook.StatusTranslated:
			if !stored || value != translation.Translation {
				return true
			}
		}
	}
	return false
}
