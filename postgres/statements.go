package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	sourceLocale = "cs-CZ"

	targetUI                  = "ui"
	targetCategory            = "category"
	targetSubcategory         = "subcategory"
	targetProduct             = "product"
	targetProductVariant      = "product_variant"
	targetCustomizationField  = "customization_field"
	targetCustomizationChoice = "customization_choice"
	targetComponent           = "component"

	targetFieldName              = "name"
	targetFieldLabel             = "label"
	targetFieldTranslation       = "translation"
	targetFieldDescription       = "description"
	targetFieldSafetyInformation = "safety_information"
	targetFieldSEOTitle          = "seo_title"
	targetFieldSEODescription    = "seo_description"
)

func upsertTranslation(
	ctx context.Context,
	db database,
	key databaseKey,
	locale, value string,
) (pgconn.CommandTag, error) {
	statement, args, err := upsertStatement(key, locale, value)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return db.Exec(ctx, statement, args...)
}

func deleteTranslation(ctx context.Context, db database, key databaseKey, locale string) (pgconn.CommandTag, error) {
	statement, args, err := deleteStatement(key, locale)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return db.Exec(ctx, statement, args...)
}

func upsertStatement(key databaseKey, locale, value string) (string, []any, error) {
	switch key.TargetKind {
	case targetUI:
		return `INSERT INTO shop.ui_translations (key, locale, translation) VALUES ($1, $2, $3)
			ON CONFLICT (key, locale) DO UPDATE SET translation = EXCLUDED.translation, updated_at = now()
			WHERE shop.ui_translations.translation IS DISTINCT FROM EXCLUDED.translation`,
			[]any{key.Name, locale, value}, nil
	case targetProductVariant:
		if locale == sourceLocale {
			return `UPDATE shop.product_variants SET name=$2, updated_at=now()
				WHERE id=$1 AND name IS DISTINCT FROM $2`, []any{key.TargetID, value}, nil
		}
		return `INSERT INTO shop.product_variant_translations(variant_id,locale,name) VALUES($1,$2,$3)
			ON CONFLICT(variant_id,locale) DO UPDATE SET name=EXCLUDED.name
			WHERE shop.product_variant_translations.name IS DISTINCT FROM EXCLUDED.name`,
			[]any{key.TargetID, locale, value}, nil
	case targetCategory:
		return `INSERT INTO shop.category_translations (category_id, locale, name) VALUES ($1, $2, $3)
			ON CONFLICT (category_id, locale) DO UPDATE SET name = EXCLUDED.name
			WHERE shop.category_translations.name IS DISTINCT FROM EXCLUDED.name`,
			[]any{key.TargetID, locale, value}, nil
	case targetSubcategory:
		return `INSERT INTO shop.subcategory_translations (subcategory_id, locale, name) VALUES ($1, $2, $3)
			ON CONFLICT (subcategory_id, locale) DO UPDATE SET name = EXCLUDED.name
			WHERE shop.subcategory_translations.name IS DISTINCT FROM EXCLUDED.name`,
			[]any{key.TargetID, locale, value}, nil
	case targetCustomizationField:
		return `INSERT INTO shop.customization_field_translations (field_id, locale, label) VALUES ($1, $2, $3)
			ON CONFLICT (field_id, locale) DO UPDATE SET label = EXCLUDED.label
			WHERE shop.customization_field_translations.label IS DISTINCT FROM EXCLUDED.label`,
			[]any{key.TargetID, locale, value}, nil
	case targetCustomizationChoice:
		return `INSERT INTO shop.customization_choice_translations (choice_id, locale, label) VALUES ($1, $2, $3)
			ON CONFLICT (choice_id, locale) DO UPDATE SET label = EXCLUDED.label
			WHERE shop.customization_choice_translations.label IS DISTINCT FROM EXCLUDED.label`,
			[]any{key.TargetID, locale, value}, nil
	case targetComponent:
		return `INSERT INTO shop.component_translations (component_id, locale, name) VALUES ($1, $2, $3)
			ON CONFLICT (component_id, locale) DO UPDATE SET name = EXCLUDED.name
			WHERE shop.component_translations.name IS DISTINCT FROM EXCLUDED.name`,
			[]any{key.TargetID, locale, value}, nil
	case targetProduct:
		return upsertProductStatement(key, locale, value)
	default:
		return "", nil, errors.New("unsupported translation target")
	}
}

func upsertProductStatement(key databaseKey, locale, value string) (string, []any, error) {
	if key.TargetField == targetFieldName {
		return `INSERT INTO shop.product_translations (product_id, locale, name) VALUES ($1, $2, $3)
			ON CONFLICT (product_id, locale) DO UPDATE SET name = EXCLUDED.name
			WHERE shop.product_translations.name IS DISTINCT FROM EXCLUDED.name`,
			[]any{key.TargetID, locale, value}, nil
	}
	statements := map[string]string{
		targetFieldDescription:       `UPDATE shop.product_translations SET description = $3 WHERE product_id = $1 AND locale = $2 AND description IS DISTINCT FROM $3`,
		targetFieldSafetyInformation: `UPDATE shop.product_translations SET safety_information = $3 WHERE product_id = $1 AND locale = $2 AND safety_information IS DISTINCT FROM $3`,
		targetFieldSEOTitle:          `UPDATE shop.product_translations SET seo_title = $3 WHERE product_id = $1 AND locale = $2 AND seo_title IS DISTINCT FROM $3`,
		targetFieldSEODescription:    `UPDATE shop.product_translations SET seo_description = $3 WHERE product_id = $1 AND locale = $2 AND seo_description IS DISTINCT FROM $3`,
	}
	statement, exists := statements[key.TargetField]
	if !exists {
		return "", nil, errors.New("unsupported translation target")
	}
	return statement, []any{key.TargetID, locale, value}, nil
}

func deleteStatement(key databaseKey, locale string) (string, []any, error) {
	switch key.TargetKind {
	case targetUI:
		return `DELETE FROM shop.ui_translations WHERE key = $1 AND locale = $2`, []any{key.Name, locale}, nil
	case targetProductVariant:
		if locale == sourceLocale {
			return "", nil, errors.New("the source variant name cannot be deleted")
		}
		return `DELETE FROM shop.product_variant_translations WHERE variant_id=$1 AND locale=$2`,
			[]any{key.TargetID, locale}, nil
	case targetCategory:
		return `DELETE FROM shop.category_translations WHERE category_id = $1 AND locale = $2`,
			[]any{key.TargetID, locale}, nil
	case targetSubcategory:
		return `DELETE FROM shop.subcategory_translations WHERE subcategory_id = $1 AND locale = $2`,
			[]any{key.TargetID, locale}, nil
	case targetCustomizationField:
		return `DELETE FROM shop.customization_field_translations WHERE field_id = $1 AND locale = $2`,
			[]any{key.TargetID, locale}, nil
	case targetCustomizationChoice:
		return `DELETE FROM shop.customization_choice_translations WHERE choice_id = $1 AND locale = $2`,
			[]any{key.TargetID, locale}, nil
	case targetComponent:
		return `DELETE FROM shop.component_translations WHERE component_id = $1 AND locale = $2`,
			[]any{key.TargetID, locale}, nil
	case targetProduct:
		return deleteProductStatement(key, locale)
	default:
		return "", nil, errors.New("unsupported translation target")
	}
}

func deleteProductStatement(key databaseKey, locale string) (string, []any, error) {
	if key.TargetField == targetFieldName {
		return `DELETE FROM shop.product_translations WHERE product_id = $1 AND locale = $2`,
			[]any{key.TargetID, locale}, nil
	}
	if key.TargetField == targetFieldDescription {
		return `UPDATE shop.product_translations SET description = '' WHERE product_id = $1 AND locale = $2 AND description <> ''`,
			[]any{key.TargetID, locale}, nil
	}
	statements := map[string]string{
		targetFieldSafetyInformation: `UPDATE shop.product_translations SET safety_information = NULL WHERE product_id = $1 AND locale = $2 AND safety_information IS NOT NULL`,
		targetFieldSEOTitle:          `UPDATE shop.product_translations SET seo_title = NULL WHERE product_id = $1 AND locale = $2 AND seo_title IS NOT NULL`,
		targetFieldSEODescription:    `UPDATE shop.product_translations SET seo_description = NULL WHERE product_id = $1 AND locale = $2 AND seo_description IS NOT NULL`,
	}
	statement, exists := statements[key.TargetField]
	if !exists {
		return "", nil, errors.New("unsupported translation target")
	}
	return statement, []any{key.TargetID, locale}, nil
}

func fieldPriority(field string) int {
	if field == targetFieldName || field == targetFieldLabel || field == targetFieldTranslation {
		return 0
	}
	return 1
}
