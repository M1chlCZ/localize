package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/m1chlcz/localize/postgres"
	"github.com/m1chlcz/localize/store"
	"github.com/m1chlcz/localize/workbook"
)

const testDatabaseURLVariable = "LOCALIZE_TEST_DATABASE_URL"

const (
	testCategoryID    = "11111111-1111-7111-8111-111111111111"
	testSubcategoryID = "22222222-2222-7222-8222-222222222222"
	testProductID     = "33333333-3333-7333-8333-333333333333"
	testVariantID     = "44444444-4444-7444-8444-444444444444"
	testFieldID       = "55555555-5555-7555-8555-555555555555"
	testChoiceID      = "66666666-6666-7666-8666-666666666666"
	testComponentID   = "77777777-7777-7777-8777-777777777777"
)

var (
	testUIKey          = "ui.test.title"
	testCategoryKey    = "catalog.category." + testCategoryID + ".name"
	testSubcategoryKey = "catalog.subcategory." + testSubcategoryID + ".name"
	testProductKey     = "catalog.product." + testProductID + ".name"
	testDescriptionKey = "catalog.product." + testProductID + ".description"
	testSeoTitleKey    = "catalog.product." + testProductID + ".seo_title"
	testFieldKey       = "catalog.customization-field." + testFieldID + ".label"
	testChoiceKey      = "catalog.customization-choice." + testChoiceID + ".label"
	testVariantKey     = "catalog.product-variant." + testVariantID + ".name"
	testComponentKey   = "catalog.component." + testComponentID + ".name"
)

var testSchemaStatements = []string{
	`DROP SCHEMA IF EXISTS shop CASCADE`,
	`CREATE SCHEMA shop`,
	`CREATE TABLE shop.localization_state (
		singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
		revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`INSERT INTO shop.localization_state (singleton, revision) VALUES (true, 1)`,
	`CREATE TABLE shop.locales (
		code text PRIMARY KEY,
		native_name text NOT NULL,
		route_prefix text NOT NULL,
		fallback_code text REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		enabled boolean NOT NULL DEFAULT false,
		is_default boolean NOT NULL DEFAULT false,
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE UNIQUE INDEX locales_one_default_idx ON shop.locales ((is_default)) WHERE is_default`,
	`CREATE TABLE shop.ui_translation_keys (
		key text PRIMARY KEY,
		context text NOT NULL DEFAULT '',
		required boolean NOT NULL DEFAULT true
	)`,
	`CREATE TABLE shop.ui_translations (
		key text NOT NULL REFERENCES shop.ui_translation_keys(key) ON UPDATE RESTRICT ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		translation text NOT NULL,
		updated_at timestamptz NOT NULL DEFAULT now(),
		CONSTRAINT ui_translations_pkey PRIMARY KEY (key, locale)
	)`,
	`CREATE TABLE shop.categories (
		id uuid PRIMARY KEY,
		slug text NOT NULL UNIQUE,
		name text NOT NULL,
		active boolean NOT NULL DEFAULT true
	)`,
	`CREATE TABLE shop.category_translations (
		category_id uuid NOT NULL REFERENCES shop.categories(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		name text NOT NULL,
		PRIMARY KEY (category_id, locale)
	)`,
	`CREATE TABLE shop.subcategories (
		id uuid PRIMARY KEY,
		category_id uuid NOT NULL REFERENCES shop.categories(id) ON DELETE RESTRICT,
		slug text NOT NULL,
		name text NOT NULL,
		active boolean NOT NULL DEFAULT true
	)`,
	`CREATE TABLE shop.subcategory_translations (
		subcategory_id uuid NOT NULL REFERENCES shop.subcategories(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		name text NOT NULL,
		PRIMARY KEY (subcategory_id, locale)
	)`,
	`CREATE TABLE shop.products (
		id uuid PRIMARY KEY,
		category_id uuid NOT NULL REFERENCES shop.categories(id) ON DELETE RESTRICT,
		slug text NOT NULL UNIQUE,
		name text NOT NULL,
		description text NOT NULL DEFAULT '',
		status text NOT NULL DEFAULT 'draft',
		safety_information text,
		seo_title text,
		seo_description text
	)`,
	`CREATE TABLE shop.product_translations (
		product_id uuid NOT NULL REFERENCES shop.products(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		name text NOT NULL,
		description text NOT NULL DEFAULT '',
		safety_information text,
		seo_title text,
		seo_description text,
		PRIMARY KEY (product_id, locale)
	)`,
	`CREATE TABLE shop.product_variants (
		id uuid PRIMARY KEY,
		product_id uuid NOT NULL REFERENCES shop.products(id) ON DELETE RESTRICT,
		sku text NOT NULL UNIQUE,
		name text NOT NULL,
		updated_at timestamptz NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE shop.product_variant_translations (
		variant_id uuid NOT NULL REFERENCES shop.product_variants(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		name text NOT NULL,
		PRIMARY KEY (variant_id, locale)
	)`,
	`CREATE TABLE shop.customization_fields (
		id uuid PRIMARY KEY,
		product_id uuid NOT NULL REFERENCES shop.products(id) ON DELETE CASCADE,
		field_key text NOT NULL,
		label text NOT NULL DEFAULT '',
		active boolean NOT NULL DEFAULT true
	)`,
	`CREATE TABLE shop.customization_field_translations (
		field_id uuid NOT NULL REFERENCES shop.customization_fields(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		label text NOT NULL,
		PRIMARY KEY (field_id, locale)
	)`,
	`CREATE TABLE shop.customization_choices (
		id uuid PRIMARY KEY,
		field_id uuid NOT NULL REFERENCES shop.customization_fields(id) ON DELETE CASCADE,
		value text NOT NULL,
		label text NOT NULL,
		active boolean NOT NULL DEFAULT true
	)`,
	`CREATE TABLE shop.customization_choice_translations (
		choice_id uuid NOT NULL REFERENCES shop.customization_choices(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		label text NOT NULL,
		PRIMARY KEY (choice_id, locale)
	)`,
	`CREATE TABLE shop.components (
		id uuid PRIMARY KEY,
		sku text NOT NULL UNIQUE,
		active boolean NOT NULL DEFAULT true
	)`,
	`CREATE TABLE shop.component_translations (
		component_id uuid NOT NULL REFERENCES shop.components(id) ON DELETE CASCADE,
		locale text NOT NULL REFERENCES shop.locales(code) ON UPDATE RESTRICT ON DELETE RESTRICT,
		name text NOT NULL,
		PRIMARY KEY (component_id, locale)
	)`,
	`CREATE FUNCTION shop.bump_localization_revision()
	RETURNS trigger
	LANGUAGE plpgsql
	SET search_path = pg_catalog, shop
	AS $function$
	BEGIN
		UPDATE shop.localization_state
		SET revision = revision + 1, updated_at = statement_timestamp()
		WHERE singleton;
		RETURN NULL;
	END
	$function$`,
	`CREATE FUNCTION shop.sync_product_variant_source_translation()
	RETURNS trigger
	LANGUAGE plpgsql
	SET search_path = pg_catalog, shop
	AS $function$
	BEGIN
		INSERT INTO shop.product_variant_translations(variant_id, locale, name)
		VALUES (NEW.id, 'cs-CZ', NEW.name)
		ON CONFLICT (variant_id, locale) DO UPDATE SET name = EXCLUDED.name;
		RETURN NEW;
	END
	$function$`,
	`CREATE TRIGGER locales_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.locales
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER category_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.category_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER subcategory_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.subcategory_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER product_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.product_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER customization_field_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.customization_field_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER customization_choice_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.customization_choice_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER component_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.component_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER product_variant_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.product_variant_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER ui_translation_keys_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.ui_translation_keys
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER ui_translations_localization_revision AFTER INSERT OR UPDATE OR DELETE ON shop.ui_translations
		FOR EACH STATEMENT EXECUTE FUNCTION shop.bump_localization_revision()`,
	`CREATE TRIGGER product_variant_source_translation AFTER INSERT OR UPDATE OF name ON shop.product_variants
		FOR EACH ROW EXECUTE FUNCTION shop.sync_product_variant_source_translation()`,
	`INSERT INTO shop.locales (code, native_name, route_prefix, fallback_code, enabled, is_default) VALUES
		('cs-CZ', 'Čeština', '', NULL, true, true),
		('en-GB', 'English', 'en', 'cs-CZ', true, false)`,
	`INSERT INTO shop.ui_translation_keys (key, context, required) VALUES ('ui.test.title', 'Test title', true)`,
	`INSERT INTO shop.ui_translations (key, locale, translation) VALUES
		('ui.test.title', 'cs-CZ', 'Titulek'),
		('ui.test.title', 'en-GB', 'Title')`,
	`INSERT INTO shop.categories (id, slug, name, active) VALUES
		('` + testCategoryID + `', 'example-category', 'Ukázková kategorie', true)`,
	`INSERT INTO shop.category_translations (category_id, locale, name) VALUES
		('` + testCategoryID + `', 'cs-CZ', 'Ukázková kategorie'),
		('` + testCategoryID + `', 'en-GB', 'Example category')`,
	`INSERT INTO shop.subcategories (id, category_id, slug, name, active) VALUES
		('` + testSubcategoryID + `', '` + testCategoryID + `', 'example-subcategory', 'Ukázková podkategorie', true)`,
	`INSERT INTO shop.subcategory_translations (subcategory_id, locale, name) VALUES
		('` + testSubcategoryID + `', 'cs-CZ', 'Ukázková podkategorie'),
		('` + testSubcategoryID + `', 'en-GB', 'Example subcategory')`,
	`INSERT INTO shop.products (id, category_id, slug, name, description, status) VALUES
		('` + testProductID + `', '` + testCategoryID + `', 'example-product', 'Ukázkový produkt', 'Ukázkový popis', 'active')`,
	`INSERT INTO shop.product_translations (product_id, locale, name, description) VALUES
		('` + testProductID + `', 'cs-CZ', 'Ukázkový produkt', 'Ukázkový popis'),
		('` + testProductID + `', 'en-GB', 'Example product', 'Example description')`,
	`INSERT INTO shop.product_variants (id, product_id, sku, name) VALUES
		('` + testVariantID + `', '` + testProductID + `', 'SKU-1', 'Ukázková varianta')`,
	`INSERT INTO shop.product_variant_translations (variant_id, locale, name) VALUES
		('` + testVariantID + `', 'en-GB', 'Example variant')`,
	`INSERT INTO shop.customization_fields (id, product_id, field_key, label, active) VALUES
		('` + testFieldID + `', '` + testProductID + `', 'size', 'Velikost', true)`,
	`INSERT INTO shop.customization_field_translations (field_id, locale, label) VALUES
		('` + testFieldID + `', 'cs-CZ', 'Velikost'),
		('` + testFieldID + `', 'en-GB', 'Size')`,
	`INSERT INTO shop.customization_choices (id, field_id, value, label, active) VALUES
		('` + testChoiceID + `', '` + testFieldID + `', 'large', 'Velká', true)`,
	`INSERT INTO shop.customization_choice_translations (choice_id, locale, label) VALUES
		('` + testChoiceID + `', 'cs-CZ', 'Velká'),
		('` + testChoiceID + `', 'en-GB', 'Large')`,
	`INSERT INTO shop.components (id, sku, active) VALUES
		('` + testComponentID + `', 'CMP-1', true)`,
	`INSERT INTO shop.component_translations (component_id, locale, name) VALUES
		('` + testComponentID + `', 'cs-CZ', 'Komponenta'),
		('` + testComponentID + `', 'en-GB', 'Component')`,
	`UPDATE shop.localization_state SET revision = 7`,
}

func TestExportMirrorsShopSchema(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if item.Version != workbook.Version || item.Revision != 7 {
		t.Fatalf("exported version = %q, revision = %d", item.Version, item.Revision)
	}
	if len(item.Locales) != 2 || item.Locales[0].Code != "cs-CZ" || !item.Locales[0].Default ||
		item.Locales[1].Code != "en-GB" || item.Locales[1].FallbackCode != "cs-CZ" {
		t.Fatalf("exported locales = %#v", item.Locales)
	}
	if len(item.Keys) != 12 || len(item.Translations) != 24 {
		t.Fatalf("keys = %d, translations = %d", len(item.Keys), len(item.Translations))
	}
	variantKey, exists := findKey(item, testVariantKey)
	if !exists || variantKey.Required || variantKey.Context != "Product variant SKU-1 name" {
		t.Fatalf("variant key = %#v, exists = %t", variantKey, exists)
	}
	categoryKey, exists := findKey(item, testCategoryKey)
	if !exists || !categoryKey.Required || categoryKey.Context != "Category example-category name" {
		t.Fatalf("category key = %#v, exists = %t", categoryKey, exists)
	}
	if row := translationRow(t, item, testVariantKey, "en-GB"); row.Translation != "Example variant" ||
		row.Status != workbook.StatusTranslated {
		t.Fatalf("variant translation = %#v", row)
	}
	if row := translationRow(t, item, testVariantKey, "cs-CZ"); row.Translation != "Ukázková varianta" ||
		row.Status != workbook.StatusTranslated {
		t.Fatalf("source variant translation = %#v", row)
	}
	if row := translationRow(t, item, testDescriptionKey, "en-GB"); row.Translation != "Example description" ||
		row.Status != workbook.StatusTranslated {
		t.Fatalf("description translation = %#v", row)
	}
	if row := translationRow(t, item, testSeoTitleKey, "en-GB"); row.Translation != "" ||
		row.Status != workbook.StatusMissing {
		t.Fatalf("seo title translation = %#v", row)
	}
}

func TestApplyIsRevisionCheckedAndRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	setTranslation(t, &item, testUIKey, "en-GB", "Improved title")
	setTranslation(t, &item, testCategoryKey, "en-GB", "Improved category")
	setTranslation(t, &item, testVariantKey, "en-GB", "Improved variant")
	setTranslation(t, &item, testVariantKey, "cs-CZ", "Vylepšená varianta")
	item.Locales[1].NativeName = "English (UK)"

	dry, err := postgres.Apply(ctx, pool, item, true)
	if err != nil {
		t.Fatalf("dry-run apply: %v", err)
	}
	if dry.Applied || dry.Changes != 5 || dry.Revision <= 7 {
		t.Fatalf("dry-run result = %#v", dry)
	}
	assertStoredValue(t, ctx, pool, item.Locales[1].Code)
	assertRowValue(t, ctx, pool, `SELECT translation FROM shop.ui_translations WHERE key = $1 AND locale = $2`,
		"Title", testUIKey, "en-GB")
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.product_variants WHERE id = $1`,
		"Ukázková varianta", testVariantID)
	assertRevision(t, ctx, pool, 7)

	result, err := postgres.Apply(ctx, pool, item, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.Applied || result.Changes != dry.Changes || result.Changes != 5 ||
		result.Revision != dry.Revision {
		t.Fatalf("apply result = %#v, dry-run = %#v", result, dry)
	}
	if result.Revision <= 7 {
		t.Fatalf("applied revision = %d", result.Revision)
	}
	assertRowValue(t, ctx, pool, `SELECT translation FROM shop.ui_translations WHERE key = $1 AND locale = $2`,
		"Improved title", testUIKey, "en-GB")
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.category_translations WHERE category_id = $1 AND locale = $2`,
		"Improved category", testCategoryID, "en-GB")
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.product_variants WHERE id = $1`,
		"Vylepšená varianta", testVariantID)
	assertRowValue(t, ctx, pool,
		`SELECT name FROM shop.product_variant_translations WHERE variant_id = $1 AND locale = $2`,
		"Vylepšená varianta", testVariantID, "cs-CZ")
	assertRowValue(t, ctx, pool,
		`SELECT name FROM shop.product_variant_translations WHERE variant_id = $1 AND locale = $2`,
		"Improved variant", testVariantID, "en-GB")
	assertRowValue(t, ctx, pool, `SELECT native_name FROM shop.locales WHERE code = $1`,
		"English (UK)", "en-GB")

	setTranslation(t, &item, testUIKey, "en-GB", "Should not stick")
	if _, staleErr := postgres.Apply(ctx, pool, item, false); !errors.Is(staleErr, store.ErrStale) {
		t.Fatalf("stale apply error = %v", staleErr)
	}
	assertRowValue(t, ctx, pool, `SELECT translation FROM shop.ui_translations WHERE key = $1 AND locale = $2`,
		"Improved title", testUIKey, "en-GB")
	assertRevision(t, ctx, pool, result.Revision)
}

func TestApplyUnchangedWorkbookIsANoOp(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	adapter := postgres.Store{Pool: pool}
	item, err := adapter.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for range 2 {
		result, applyErr := adapter.Apply(ctx, item, false)
		if applyErr != nil {
			t.Fatalf("no-op apply: %v", applyErr)
		}
		if !result.Applied || result.Changes != 0 || result.Revision != 7 {
			t.Fatalf("no-op result = %#v", result)
		}
	}
	setTranslation(t, &item, testUIKey, "en-GB", "Changed title")
	result, err := adapter.Apply(ctx, item, false)
	if err != nil {
		t.Fatalf("changing apply: %v", err)
	}
	if result.Changes == 0 || result.Revision <= 7 {
		t.Fatalf("changing result = %#v", result)
	}
	if _, staleErr := adapter.Apply(ctx, item, false); !errors.Is(staleErr, store.ErrStale) {
		t.Fatalf("stale apply error = %v", staleErr)
	}
}

func TestApplyDeletesOptionalTranslations(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	deleteRow(t, &item, testDescriptionKey, "en-GB")
	deleteRow(t, &item, testVariantKey, "en-GB")
	result, err := postgres.Apply(ctx, pool, item, false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Changes != 2 {
		t.Fatalf("apply result = %#v", result)
	}
	assertRowValue(t, ctx, pool,
		`SELECT description FROM shop.product_translations WHERE product_id = $1 AND locale = $2`,
		"", testProductID, "en-GB")
	assertRowCount(t, ctx, pool,
		`SELECT count(*) FROM shop.product_variant_translations WHERE variant_id = $1 AND locale = $2`,
		0, testVariantID, "en-GB")
	fresh, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("fresh export: %v", err)
	}
	if row := translationRow(t, fresh, testDescriptionKey, "en-GB"); row.Status != workbook.StatusMissing {
		t.Fatalf("deleted description = %#v", row)
	}
	if row := translationRow(t, fresh, testVariantKey, "en-GB"); row.Status != workbook.StatusMissing {
		t.Fatalf("deleted variant = %#v", row)
	}
}

func TestApplyRejectsDeletingRequiredTranslation(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	deleteRow(t, &item, testCategoryKey, "en-GB")
	if _, applyErr := postgres.Apply(ctx, pool, item, false); applyErr == nil ||
		!strings.Contains(applyErr.Error(), "missing required key") {
		t.Fatalf("apply error = %v", applyErr)
	}
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.category_translations WHERE category_id = $1 AND locale = $2`,
		"Example category", testCategoryID, "en-GB")
	assertRevision(t, ctx, pool, 7)
}

func TestApplyFailureRollsBackEveryEarlierChange(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	setTranslation(t, &item, testCategoryKey, "en-GB", "Should not stick")
	row := translationRow(t, item, testVariantKey, "cs-CZ")
	row.Translation = ""
	row.Status = workbook.StatusDelete
	if _, applyErr := postgres.Apply(ctx, pool, item, false); applyErr == nil ||
		!strings.Contains(applyErr.Error(), "source variant name cannot be deleted") {
		t.Fatalf("apply error = %v", applyErr)
	}
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.category_translations WHERE category_id = $1 AND locale = $2`,
		"Example category", testCategoryID, "en-GB")
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.product_variants WHERE id = $1`,
		"Ukázková varianta", testVariantID)
	assertRevision(t, ctx, pool, 7)
}

func TestApplyRejectsChangedMetadata(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	key := findKeyAt(t, item, testCategoryKey)
	item.Keys[key].Context = "Other context"
	for index := range item.Translations {
		if item.Translations[index].Key == testCategoryKey {
			item.Translations[index].Context = "Other context"
		}
	}
	if _, applyErr := postgres.Apply(ctx, pool, item, false); applyErr == nil ||
		!strings.Contains(applyErr.Error(), "does not match the database") {
		t.Fatalf("apply error = %v", applyErr)
	}
	assertRevision(t, ctx, pool, 7)
}

func TestApplyRejectsOmittedLocale(t *testing.T) {
	ctx := context.Background()
	pool := integrationPool(t, ctx)
	item, err := postgres.Export(ctx, pool)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	item.Locales = item.Locales[:1]
	translations := item.Translations[:0]
	for _, translation := range item.Translations {
		if translation.Locale == "cs-CZ" {
			translations = append(translations, translation)
		}
	}
	item.Translations = translations
	if _, applyErr := postgres.Apply(ctx, pool, item, false); applyErr == nil ||
		!strings.Contains(applyErr.Error(), "omits existing locale en-GB") {
		t.Fatalf("apply error = %v", applyErr)
	}
	assertRevision(t, ctx, pool, 7)
}

func integrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(testDatabaseURLVariable))
	if dsn == "" {
		t.Skipf("set %s to run PostgreSQL integration tests", testDatabaseURLVariable)
	}
	pool, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(pool.Close)
	for _, statement := range testSchemaStatements {
		if _, err = pool.Exec(ctx, statement); err != nil {
			t.Fatalf("apply test schema: %v", err)
		}
	}
	return pool
}

func findKey(item workbook.Workbook, name string) (workbook.Key, bool) {
	for _, key := range item.Keys {
		if key.Name == name {
			return key, true
		}
	}
	return workbook.Key{}, false
}

func findKeyAt(t *testing.T, item workbook.Workbook, name string) int {
	t.Helper()
	for index, key := range item.Keys {
		if key.Name == name {
			return index
		}
	}
	t.Fatalf("key %s is missing", name)
	return 0
}

func translationRow(t *testing.T, item workbook.Workbook, key, locale string) *workbook.Translation {
	t.Helper()
	for index := range item.Translations {
		row := &item.Translations[index]
		if row.Key == key && row.Locale == locale {
			return row
		}
	}
	t.Fatalf("translation %s/%s is missing", key, locale)
	return nil
}

func setTranslation(t *testing.T, item *workbook.Workbook, key, locale, value string) {
	t.Helper()
	for index := range item.Translations {
		row := &item.Translations[index]
		if row.Key == key && row.Locale == locale {
			row.Translation = value
			row.Status = workbook.StatusTranslated
			return
		}
	}
	t.Fatalf("translation %s/%s is missing", key, locale)
}

func assertRevision(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var revision int64
	if err := pool.QueryRow(ctx, `SELECT revision FROM shop.localization_state WHERE singleton`).Scan(&revision); err != nil {
		t.Fatalf("read revision: %v", err)
	}
	if revision != want {
		t.Fatalf("revision = %d, want %d", revision, want)
	}
}

func deleteRow(t *testing.T, item *workbook.Workbook, key, locale string) {
	t.Helper()
	for index := range item.Translations {
		row := &item.Translations[index]
		if row.Key == key && row.Locale == locale {
			row.Translation = ""
			row.Status = workbook.StatusDelete
			return
		}
	}
	t.Fatalf("translation %s/%s is missing", key, locale)
}

func assertRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, want int64, args ...any) {
	t.Helper()
	var got int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if got != want {
		t.Fatalf("row count = %d, want %d", got, want)
	}
}

func assertRowValue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query, want string, args ...any) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if got != want {
		t.Fatalf("row value = %q, want %q", got, want)
	}
}

func assertStoredValue(t *testing.T, ctx context.Context, pool *pgxpool.Pool, locale string) {
	t.Helper()
	assertRowValue(t, ctx, pool, `SELECT native_name FROM shop.locales WHERE code = $1`, "English", locale)
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.product_variants WHERE id = $1`, "Ukázková varianta", testVariantID)
	assertRowValue(t, ctx, pool, `SELECT name FROM shop.category_translations WHERE category_id = $1 AND locale = $2`,
		"Example category", testCategoryID, "en-GB")
}
