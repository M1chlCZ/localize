package postgres

import (
	"reflect"
	"strings"
	"testing"
)

func TestQueriesCoverEveryLocalizedTable(t *testing.T) {
	t.Parallel()
	keyTables := []string{
		"shop.ui_translation_keys",
		"shop.categories",
		"shop.subcategories",
		"shop.products",
		"shop.customization_fields",
		"shop.customization_choices",
		"shop.product_variants",
		"shop.components",
	}
	for _, table := range keyTables {
		if !strings.Contains(loadKeysSQL, table) {
			t.Fatalf("loadKeysSQL does not read %s", table)
		}
	}
	translationTables := []string{
		"shop.ui_translations",
		"shop.category_translations",
		"shop.subcategory_translations",
		"shop.product_translations",
		"shop.customization_field_translations",
		"shop.customization_choice_translations",
		"shop.product_variant_translations",
		"shop.component_translations",
	}
	for _, table := range translationTables {
		if !strings.Contains(loadTranslationsSQL, table) {
			t.Fatalf("loadTranslationsSQL does not read %s", table)
		}
	}
	requiredTables := []string{
		"shop.ui_translations",
		"shop.category_translations",
		"shop.subcategory_translations",
		"shop.product_translations",
		"shop.customization_field_translations",
		"shop.customization_choice_translations",
		"shop.component_translations",
	}
	for _, table := range requiredTables {
		if !strings.Contains(missingRequiredSQL, table) {
			t.Fatalf("missingRequiredSQL does not check %s", table)
		}
	}
	if !strings.Contains(loadLocalesSQL, "shop.locales") || !strings.Contains(revisionSQL, "shop.localization_state") {
		t.Fatal("locale or revision query does not read the shop schema")
	}
}

func TestUpsertStatementTargetsShopTables(t *testing.T) {
	t.Parallel()
	productID := "33333333-3333-7333-8333-333333333333"
	for _, test := range []struct {
		name     string
		key      databaseKey
		locale   string
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "ui translation",
			key:      databaseKey{Name: "ui.test.title", TargetKind: targetUI, TargetField: targetFieldTranslation},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.ui_translations (key, locale, translation) VALUES ($1, $2, $3) ON CONFLICT (key, locale) DO UPDATE SET translation = EXCLUDED.translation, updated_at = now() WHERE shop.ui_translations.translation IS DISTINCT FROM EXCLUDED.translation`,
			wantArgs: []any{"ui.test.title", "en-GB", "Title"},
		},
		{
			name:     "source product variant name",
			key:      databaseKey{Name: "variant", TargetKind: targetProductVariant, TargetID: productID, TargetField: targetFieldName},
			locale:   sourceLocale,
			wantSQL:  `UPDATE shop.product_variants SET name=$2, updated_at=now() WHERE id=$1 AND name IS DISTINCT FROM $2`,
			wantArgs: []any{productID, "Varianta"},
		},
		{
			name:     "translated product variant name",
			key:      databaseKey{Name: "variant", TargetKind: targetProductVariant, TargetID: productID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.product_variant_translations(variant_id,locale,name) VALUES($1,$2,$3) ON CONFLICT(variant_id,locale) DO UPDATE SET name=EXCLUDED.name WHERE shop.product_variant_translations.name IS DISTINCT FROM EXCLUDED.name`,
			wantArgs: []any{productID, "en-GB", "Variant"},
		},
		{
			name:     "category name",
			key:      databaseKey{Name: "category", TargetKind: targetCategory, TargetID: productID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.category_translations (category_id, locale, name) VALUES ($1, $2, $3) ON CONFLICT (category_id, locale) DO UPDATE SET name = EXCLUDED.name WHERE shop.category_translations.name IS DISTINCT FROM EXCLUDED.name`,
			wantArgs: []any{productID, "en-GB", "Category"},
		},
		{
			name:     "subcategory name",
			key:      databaseKey{Name: "subcategory", TargetKind: targetSubcategory, TargetID: productID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.subcategory_translations (subcategory_id, locale, name) VALUES ($1, $2, $3) ON CONFLICT (subcategory_id, locale) DO UPDATE SET name = EXCLUDED.name WHERE shop.subcategory_translations.name IS DISTINCT FROM EXCLUDED.name`,
			wantArgs: []any{productID, "en-GB", "Subcategory"},
		},
		{
			name:     "customization field label",
			key:      databaseKey{Name: "field", TargetKind: targetCustomizationField, TargetID: productID, TargetField: targetFieldLabel},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.customization_field_translations (field_id, locale, label) VALUES ($1, $2, $3) ON CONFLICT (field_id, locale) DO UPDATE SET label = EXCLUDED.label WHERE shop.customization_field_translations.label IS DISTINCT FROM EXCLUDED.label`,
			wantArgs: []any{productID, "en-GB", "Label"},
		},
		{
			name:     "customization choice label",
			key:      databaseKey{Name: "choice", TargetKind: targetCustomizationChoice, TargetID: productID, TargetField: targetFieldLabel},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.customization_choice_translations (choice_id, locale, label) VALUES ($1, $2, $3) ON CONFLICT (choice_id, locale) DO UPDATE SET label = EXCLUDED.label WHERE shop.customization_choice_translations.label IS DISTINCT FROM EXCLUDED.label`,
			wantArgs: []any{productID, "en-GB", "Label"},
		},
		{
			name:     "component name",
			key:      databaseKey{Name: "component", TargetKind: targetComponent, TargetID: productID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.component_translations (component_id, locale, name) VALUES ($1, $2, $3) ON CONFLICT (component_id, locale) DO UPDATE SET name = EXCLUDED.name WHERE shop.component_translations.name IS DISTINCT FROM EXCLUDED.name`,
			wantArgs: []any{productID, "en-GB", "Component"},
		},
		{
			name:     "product name",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: productID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `INSERT INTO shop.product_translations (product_id, locale, name) VALUES ($1, $2, $3) ON CONFLICT (product_id, locale) DO UPDATE SET name = EXCLUDED.name WHERE shop.product_translations.name IS DISTINCT FROM EXCLUDED.name`,
			wantArgs: []any{productID, "en-GB", "Product"},
		},
		{
			name:     "product description",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: productID, TargetField: targetFieldDescription},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET description = $3 WHERE product_id = $1 AND locale = $2 AND description IS DISTINCT FROM $3`,
			wantArgs: []any{productID, "en-GB", "Description"},
		},
		{
			name:     "product safety information",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: productID, TargetField: targetFieldSafetyInformation},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET safety_information = $3 WHERE product_id = $1 AND locale = $2 AND safety_information IS DISTINCT FROM $3`,
			wantArgs: []any{productID, "en-GB", "Safety"},
		},
		{
			name:     "product seo title",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: productID, TargetField: targetFieldSEOTitle},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET seo_title = $3 WHERE product_id = $1 AND locale = $2 AND seo_title IS DISTINCT FROM $3`,
			wantArgs: []any{productID, "en-GB", "SEO title"},
		},
		{
			name:     "product seo description",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: productID, TargetField: targetFieldSEODescription},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET seo_description = $3 WHERE product_id = $1 AND locale = $2 AND seo_description IS DISTINCT FROM $3`,
			wantArgs: []any{productID, "en-GB", "SEO description"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := test.wantArgs[len(test.wantArgs)-1].(string)
			statement, args, err := upsertStatement(test.key, test.locale, value)
			if err != nil {
				t.Fatalf("upsert statement: %v", err)
			}
			if got := normalizeStatement(statement); got != test.wantSQL {
				t.Fatalf("statement = %q, want %q", got, test.wantSQL)
			}
			if !reflect.DeepEqual(args, test.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
			}
		})
	}
}

func TestUpsertStatementRejectsUnknownTargets(t *testing.T) {
	t.Parallel()
	for _, key := range []databaseKey{
		{Name: "key", TargetKind: "unknown", TargetField: targetFieldTranslation},
		{Name: "key", TargetKind: targetProduct, TargetField: "unknown"},
	} {
		if _, _, err := upsertStatement(key, "en-GB", "Value"); err == nil {
			t.Fatalf("upsert accepted target %#v", key)
		}
	}
}

func TestDeleteStatementTargetsShopTables(t *testing.T) {
	t.Parallel()
	targetID := "33333333-3333-7333-8333-333333333333"
	for _, test := range []struct {
		name     string
		key      databaseKey
		locale   string
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "ui translation",
			key:      databaseKey{Name: "ui.test.title", TargetKind: targetUI, TargetField: targetFieldTranslation},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.ui_translations WHERE key = $1 AND locale = $2`,
			wantArgs: []any{"ui.test.title", "en-GB"},
		},
		{
			name:     "translated product variant name",
			key:      databaseKey{Name: "variant", TargetKind: targetProductVariant, TargetID: targetID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.product_variant_translations WHERE variant_id=$1 AND locale=$2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "category name",
			key:      databaseKey{Name: "category", TargetKind: targetCategory, TargetID: targetID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.category_translations WHERE category_id = $1 AND locale = $2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "subcategory name",
			key:      databaseKey{Name: "subcategory", TargetKind: targetSubcategory, TargetID: targetID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.subcategory_translations WHERE subcategory_id = $1 AND locale = $2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "customization field label",
			key:      databaseKey{Name: "field", TargetKind: targetCustomizationField, TargetID: targetID, TargetField: targetFieldLabel},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.customization_field_translations WHERE field_id = $1 AND locale = $2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "customization choice label",
			key:      databaseKey{Name: "choice", TargetKind: targetCustomizationChoice, TargetID: targetID, TargetField: targetFieldLabel},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.customization_choice_translations WHERE choice_id = $1 AND locale = $2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "component name",
			key:      databaseKey{Name: "component", TargetKind: targetComponent, TargetID: targetID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.component_translations WHERE component_id = $1 AND locale = $2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "product name",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: targetID, TargetField: targetFieldName},
			locale:   "en-GB",
			wantSQL:  `DELETE FROM shop.product_translations WHERE product_id = $1 AND locale = $2`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "product description",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: targetID, TargetField: targetFieldDescription},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET description = '' WHERE product_id = $1 AND locale = $2 AND description <> ''`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "product safety information",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: targetID, TargetField: targetFieldSafetyInformation},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET safety_information = NULL WHERE product_id = $1 AND locale = $2 AND safety_information IS NOT NULL`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "product seo title",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: targetID, TargetField: targetFieldSEOTitle},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET seo_title = NULL WHERE product_id = $1 AND locale = $2 AND seo_title IS NOT NULL`,
			wantArgs: []any{targetID, "en-GB"},
		},
		{
			name:     "product seo description",
			key:      databaseKey{Name: "product", TargetKind: targetProduct, TargetID: targetID, TargetField: targetFieldSEODescription},
			locale:   "en-GB",
			wantSQL:  `UPDATE shop.product_translations SET seo_description = NULL WHERE product_id = $1 AND locale = $2 AND seo_description IS NOT NULL`,
			wantArgs: []any{targetID, "en-GB"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			statement, args, err := deleteStatement(test.key, test.locale)
			if err != nil {
				t.Fatalf("delete statement: %v", err)
			}
			if got := normalizeStatement(statement); got != test.wantSQL {
				t.Fatalf("statement = %q, want %q", got, test.wantSQL)
			}
			if !reflect.DeepEqual(args, test.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
			}
		})
	}
}

func TestDeleteStatementRejectsProtectedTargets(t *testing.T) {
	t.Parallel()
	variant := databaseKey{
		Name: "variant", TargetKind: targetProductVariant,
		TargetID: "33333333-3333-7333-8333-333333333333", TargetField: targetFieldName,
	}
	if _, _, err := deleteStatement(variant, sourceLocale); err == nil {
		t.Fatal("delete accepted the source variant name")
	}
	for _, key := range []databaseKey{
		{Name: "key", TargetKind: "unknown", TargetField: targetFieldTranslation},
		{Name: "key", TargetKind: targetProduct, TargetField: "unknown"},
	} {
		if _, _, err := deleteStatement(key, "en-GB"); err == nil {
			t.Fatalf("delete accepted target %#v", key)
		}
	}
}

func TestFieldPriorityKeepsPrimaryFieldsFirst(t *testing.T) {
	t.Parallel()
	for _, field := range []string{targetFieldName, targetFieldLabel, targetFieldTranslation} {
		if priority := fieldPriority(field); priority != 0 {
			t.Fatalf("fieldPriority(%q) = %d, want 0", field, priority)
		}
	}
	for _, field := range []string{
		targetFieldDescription,
		targetFieldSafetyInformation,
		targetFieldSEOTitle,
		targetFieldSEODescription,
		"unknown",
	} {
		if priority := fieldPriority(field); priority != 1 {
			t.Fatalf("fieldPriority(%q) = %d, want 1", field, priority)
		}
	}
}

func normalizeStatement(statement string) string {
	return strings.Join(strings.Fields(statement), " ")
}
