// Package postgres reads and applies localization workbooks against a
// PostgreSQL database that uses the shop localization schema.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/m1chlcz/localize/store"
	"github.com/m1chlcz/localize/workbook"
)

const (
	exportedKeyCapacity         = 256
	exportedTranslationCapacity = 512
)

const revisionSQL = `SELECT revision FROM shop.localization_state WHERE singleton`

const lockedRevisionSQL = revisionSQL + ` FOR UPDATE`

const loadLocalesSQL = `
	SELECT code, native_name, route_prefix, fallback_code, enabled, is_default
	FROM shop.locales ORDER BY code
`

const loadKeysSQL = `
	SELECT key, 'ui', '', 'translation', context, required FROM shop.ui_translation_keys
	UNION ALL
	SELECT 'catalog.category.' || id || '.name', 'category', id::text, 'name',
	       'Category ' || slug || ' name', active FROM shop.categories
	UNION ALL
	SELECT 'catalog.subcategory.' || id || '.name', 'subcategory', id::text, 'name',
	       'Subcategory ' || slug || ' name', active FROM shop.subcategories
	UNION ALL
	SELECT 'catalog.product.' || id || '.name', 'product', id::text, 'name',
	       'Product ' || slug || ' name', status = 'active' FROM shop.products
	UNION ALL
	SELECT 'catalog.product.' || id || '.description', 'product', id::text, 'description',
	       'Product ' || slug || ' description', false FROM shop.products
	UNION ALL
	SELECT 'catalog.product.' || id || '.safety_information', 'product', id::text, 'safety_information',
	       'Product ' || slug || ' safety information', false FROM shop.products
	UNION ALL
	SELECT 'catalog.product.' || id || '.seo_title', 'product', id::text, 'seo_title',
	       'Product ' || slug || ' SEO title', false FROM shop.products
	UNION ALL
	SELECT 'catalog.product.' || id || '.seo_description', 'product', id::text, 'seo_description',
	       'Product ' || slug || ' SEO description', false FROM shop.products
	UNION ALL
	SELECT 'catalog.customization-field.' || field.id || '.label', 'customization_field', field.id::text,
	       'label', 'Configurator field ' || field.field_key || ' label', field.active AND product.status = 'active'
	FROM shop.customization_fields AS field JOIN shop.products AS product ON product.id = field.product_id
	UNION ALL
	SELECT 'catalog.customization-choice.' || choice.id || '.label', 'customization_choice', choice.id::text,
	       'label', 'Configurator choice ' || choice.value || ' label',
	       choice.active AND field.active AND product.status = 'active'
	FROM shop.customization_choices AS choice
	JOIN shop.customization_fields AS field ON field.id = choice.field_id
	JOIN shop.products AS product ON product.id = field.product_id
	UNION ALL
	SELECT 'catalog.product-variant.' || id || '.name', 'product_variant', id::text, 'name',
	       'Product variant ' || sku || ' name', false FROM shop.product_variants
	UNION ALL
	SELECT 'catalog.component.' || id || '.name', 'component', id::text, 'name',
	       'Warehouse component ' || sku || ' customer name', active FROM shop.components
	ORDER BY 1
`

const loadTranslationsSQL = `
	SELECT key, locale, translation FROM shop.ui_translations
	UNION ALL SELECT 'catalog.category.' || category_id || '.name', locale, name FROM shop.category_translations
	UNION ALL SELECT 'catalog.subcategory.' || subcategory_id || '.name', locale, name FROM shop.subcategory_translations
	UNION ALL SELECT 'catalog.product.' || product_id || '.name', locale, name FROM shop.product_translations
	UNION ALL SELECT 'catalog.product.' || product_id || '.description', locale, description
	  FROM shop.product_translations WHERE description <> ''
	UNION ALL SELECT 'catalog.product.' || product_id || '.safety_information', locale, safety_information
	  FROM shop.product_translations WHERE safety_information IS NOT NULL AND safety_information <> ''
	UNION ALL SELECT 'catalog.product.' || product_id || '.seo_title', locale, seo_title
	  FROM shop.product_translations WHERE seo_title IS NOT NULL AND seo_title <> ''
	UNION ALL SELECT 'catalog.product.' || product_id || '.seo_description', locale, seo_description
	  FROM shop.product_translations WHERE seo_description IS NOT NULL AND seo_description <> ''
	UNION ALL SELECT 'catalog.customization-field.' || field_id || '.label', locale, label
	  FROM shop.customization_field_translations WHERE label <> ''
	UNION ALL SELECT 'catalog.customization-choice.' || choice_id || '.label', locale, label
	  FROM shop.customization_choice_translations
	UNION ALL SELECT 'catalog.product-variant.' || variant_id || '.name', locale, name
	  FROM shop.product_variant_translations
	UNION ALL SELECT 'catalog.component.' || component_id || '.name', locale, name
	  FROM shop.component_translations
`

const missingRequiredSQL = `
	SELECT
	(SELECT count(*) FROM shop.ui_translation_keys AS item WHERE item.required AND NOT EXISTS (
		SELECT FROM shop.ui_translations AS translation WHERE translation.key = item.key AND translation.locale = $1)) +
	(SELECT count(*) FROM shop.categories AS item WHERE item.active AND NOT EXISTS (
		SELECT FROM shop.category_translations AS translation WHERE translation.category_id = item.id AND translation.locale = $1)) +
	(SELECT count(*) FROM shop.subcategories AS item WHERE item.active AND NOT EXISTS (
		SELECT FROM shop.subcategory_translations AS translation WHERE translation.subcategory_id = item.id AND translation.locale = $1)) +
	(SELECT count(*) FROM shop.products AS item WHERE item.status = 'active' AND NOT EXISTS (
		SELECT FROM shop.product_translations AS translation WHERE translation.product_id = item.id AND translation.locale = $1)) +
	(SELECT count(*) FROM shop.customization_fields AS item JOIN shop.products AS product ON product.id = item.product_id
	 WHERE item.active AND product.status = 'active' AND NOT EXISTS (
		SELECT FROM shop.customization_field_translations AS translation WHERE translation.field_id = item.id AND translation.locale = $1)) +
	(SELECT count(*) FROM shop.customization_choices AS item
	 JOIN shop.customization_fields AS field ON field.id = item.field_id JOIN shop.products AS product ON product.id = field.product_id
	 WHERE item.active AND field.active AND product.status = 'active' AND NOT EXISTS (
		SELECT FROM shop.customization_choice_translations AS translation WHERE translation.choice_id = item.id AND translation.locale = $1)) +
	(SELECT count(*) FROM shop.components AS item WHERE item.active AND NOT EXISTS (
		SELECT FROM shop.component_translations AS translation WHERE translation.component_id = item.id AND translation.locale = $1))
`

// ErrStale reports that the workbook revision does not match the database
// revision. Apply returns an error that wraps this sentinel.
var ErrStale = store.ErrStale

// Store reads and applies workbooks through a PostgreSQL pool. It implements
// the store.Store interface.
type Store struct {
	Pool *pgxpool.Pool
}

var _ store.Store = Store{}

// Open connects to the database and returns a pool. The caller must close it.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open localization database: %w", err)
	}
	if pingErr := pool.Ping(ctx); pingErr != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to localization database: %w", pingErr)
	}
	return pool, nil
}

// Snapshot reads every locale, key, and translation into a workbook.
func (s Store) Snapshot(ctx context.Context) (workbook.Workbook, error) {
	return Export(ctx, s.Pool)
}

// Apply validates the workbook and applies it in one transaction.
func (s Store) Apply(
	ctx context.Context,
	incoming workbook.Workbook,
	dryRun bool,
) (store.Result, error) {
	return Apply(ctx, s.Pool, incoming, dryRun)
}

// Export reads every locale, key, and translation in one repeatable-read
// read-only transaction.
func Export(ctx context.Context, pool *pgxpool.Pool) (workbook.Workbook, error) {
	if pool == nil {
		return workbook.Workbook{}, errors.New("database pool is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return workbook.Workbook{}, fmt.Errorf("begin localization export: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	item, err := loadWorkbook(ctx, tx)
	if err != nil {
		return workbook.Workbook{}, err
	}
	item.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	if validateErr := workbook.Validate(item); validateErr != nil {
		return workbook.Workbook{}, fmt.Errorf("database localization snapshot is invalid: %w", validateErr)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return workbook.Workbook{}, fmt.Errorf("commit localization export: %w", commitErr)
	}
	return item, nil
}

// Apply validates the workbook and applies it in one serializable transaction
// under the shop import advisory lock. A stale revision is rejected with an
// error that wraps ErrStale. A dry run reports the changes and rolls back. An
// incoming workbook that matches the stored state executes no statements,
// reports zero changes, and leaves the revision in place, so it can be applied
// again; a workbook with changes advances the revision, so it becomes stale.
func Apply(
	ctx context.Context,
	pool *pgxpool.Pool,
	incoming workbook.Workbook,
	dryRun bool,
) (store.Result, error) {
	if pool == nil {
		return store.Result{}, errors.New("database pool is required")
	}
	if err := workbook.Validate(incoming); err != nil {
		return store.Result{}, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return store.Result{}, fmt.Errorf("begin localization import: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, execErr := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('shop.localization.import'))`,
	); execErr != nil {
		return store.Result{}, fmt.Errorf("lock localization import: %w", execErr)
	}
	var currentRevision int64
	if scanErr := tx.QueryRow(
		ctx,
		lockedRevisionSQL,
	).Scan(&currentRevision); scanErr != nil {
		return store.Result{}, fmt.Errorf("read localization revision: %w", scanErr)
	}
	if currentRevision != incoming.Revision {
		return store.Result{}, fmt.Errorf(
			"%w: workbook=%d database=%d",
			ErrStale,
			incoming.Revision,
			currentRevision,
		)
	}
	changes, revision, err := applyWorkbookTx(ctx, tx, incoming)
	if err != nil {
		return store.Result{}, err
	}
	result := store.Result{Applied: !dryRun, Changes: changes, Revision: revision}
	if dryRun {
		if err = tx.Rollback(ctx); err != nil {
			return store.Result{}, fmt.Errorf("rollback localization dry-run: %w", err)
		}
		return result, nil
	}
	if err = tx.Commit(ctx); err != nil {
		return store.Result{}, fmt.Errorf("commit localization import: %w", err)
	}
	return result, nil
}

type database interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type databaseKey struct {
	Name        string
	TargetKind  string
	TargetID    string
	TargetField string
	Context     string
	Required    bool
}

func (k databaseKey) workbookKey() workbook.Key {
	return workbook.Key{Name: k.Name, Context: k.Context, Required: k.Required}
}

func applyWorkbookTx(ctx context.Context, tx pgx.Tx, incoming workbook.Workbook) (int64, int64, error) {
	authoritativeKeys, err := loadKeys(ctx, tx)
	if err != nil {
		return 0, 0, err
	}
	if !sameKeys(authoritativeKeys, incoming.Keys) {
		return 0, 0, errors.New("localization workbook metadata does not match the database")
	}
	if validateLocaleSetErr := validateLocaleSet(ctx, tx, incoming.Locales); validateLocaleSetErr != nil {
		return 0, 0, validateLocaleSetErr
	}
	var changes int64
	differs, err := workbookDiffers(ctx, tx, incoming)
	if err != nil {
		return 0, 0, err
	}
	if differs {
		localeChanges, localeErr := applyLocales(ctx, tx, incoming.Locales)
		if localeErr != nil {
			return 0, 0, localeErr
		}
		translationChanges, translationErr := applyTranslations(ctx, tx, incoming, authoritativeKeys)
		if translationErr != nil {
			return 0, 0, translationErr
		}
		changes = localeChanges + translationChanges
	}
	if err = validateEnabledLocaleTranslations(ctx, tx, incoming.Locales); err != nil {
		return 0, 0, err
	}
	var revision int64
	if err = tx.QueryRow(ctx, revisionSQL).Scan(&revision); err != nil {
		return 0, 0, fmt.Errorf("read imported localization revision: %w", err)
	}
	return changes, revision, nil
}

func workbookDiffers(ctx context.Context, db database, incoming workbook.Workbook) (bool, error) {
	currentLocales, err := loadLocales(ctx, db)
	if err != nil {
		return false, err
	}
	if localesDiffer(currentLocales, incoming.Locales) {
		return true, nil
	}
	actual, err := loadActualTranslations(ctx, db)
	if err != nil {
		return false, err
	}
	return translationsDiffer(incoming.Translations, actual), nil
}

func validateEnabledLocaleTranslations(ctx context.Context, tx pgx.Tx, locales []workbook.Locale) error {
	for _, locale := range locales {
		if !locale.Enabled {
			continue
		}
		missing, err := missingRequiredCount(ctx, tx, locale.Code)
		if err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf(
				"enabled locale %s is missing %d required database translations",
				locale.Code,
				missing,
			)
		}
	}
	return nil
}

func loadWorkbook(ctx context.Context, db database) (workbook.Workbook, error) {
	item := workbook.Workbook{Version: workbook.Version}
	if err := db.QueryRow(ctx, revisionSQL).Scan(&item.Revision); err != nil {
		return workbook.Workbook{}, fmt.Errorf("read localization revision: %w", err)
	}
	locales, err := loadLocales(ctx, db)
	if err != nil {
		return workbook.Workbook{}, err
	}
	item.Locales = locales
	keys, err := loadKeys(ctx, db)
	if err != nil {
		return workbook.Workbook{}, err
	}
	item.Keys = make([]workbook.Key, 0, len(keys))
	for _, key := range keys {
		item.Keys = append(item.Keys, key.workbookKey())
	}
	actual, err := loadActualTranslations(ctx, db)
	if err != nil {
		return workbook.Workbook{}, err
	}
	for _, key := range keys {
		for _, locale := range locales {
			value, stored := actual[translationIdentity(key.Name, locale.Code)]
			item.Translations = append(
				item.Translations,
				translationFromStored(key, locale.Code, value, stored),
			)
		}
	}
	return item, nil
}

func loadLocales(ctx context.Context, db database) ([]workbook.Locale, error) {
	rows, err := db.Query(ctx, loadLocalesSQL)
	if err != nil {
		return nil, fmt.Errorf("list localization languages: %w", err)
	}
	defer rows.Close()
	locales := make([]workbook.Locale, 0, 8)
	for rows.Next() {
		var code, nativeName, routePrefix string
		var fallback *string
		var enabled, isDefault bool
		if scanErr := rows.Scan(
			&code, &nativeName, &routePrefix, &fallback, &enabled, &isDefault,
		); scanErr != nil {
			return nil, fmt.Errorf("scan localization language: %w", scanErr)
		}
		locales = append(locales, localeFromRow(code, nativeName, routePrefix, fallback, enabled, isDefault))
	}
	if errCause := rows.Err(); errCause != nil {
		return nil, fmt.Errorf("list localization languages: %w", errCause)
	}
	return locales, nil
}

func loadKeys(ctx context.Context, db database) ([]databaseKey, error) {
	rows, err := db.Query(ctx, loadKeysSQL)
	if err != nil {
		return nil, fmt.Errorf("list localization keys: %w", err)
	}
	defer rows.Close()
	keys := make([]databaseKey, 0, exportedKeyCapacity)
	for rows.Next() {
		var key databaseKey
		if scanErr := rows.Scan(
			&key.Name,
			&key.TargetKind,
			&key.TargetID,
			&key.TargetField,
			&key.Context,
			&key.Required,
		); scanErr != nil {
			return nil, fmt.Errorf("scan localization key: %w", scanErr)
		}
		keys = append(keys, key)
	}
	if errCause := rows.Err(); errCause != nil {
		return nil, fmt.Errorf("list localization keys: %w", errCause)
	}
	return keys, nil
}

func loadActualTranslations(ctx context.Context, db database) (map[string]string, error) {
	rows, err := db.Query(ctx, loadTranslationsSQL)
	if err != nil {
		return nil, fmt.Errorf("list localization translations: %w", err)
	}
	defer rows.Close()
	translations := make(map[string]string, exportedTranslationCapacity)
	for rows.Next() {
		var key, locale, translation string
		if scanErr := rows.Scan(&key, &locale, &translation); scanErr != nil {
			return nil, fmt.Errorf("scan localization translation: %w", scanErr)
		}
		translations[translationIdentity(key, locale)] = translation
	}
	if errCause := rows.Err(); errCause != nil {
		return nil, fmt.Errorf("list localization translations: %w", errCause)
	}
	return translations, nil
}

func validateLocaleSet(ctx context.Context, db database, workbookLocales []workbook.Locale) error {
	rows, err := db.Query(ctx, `SELECT code, is_default FROM shop.locales ORDER BY code`)
	if err != nil {
		return fmt.Errorf("read database locale set: %w", err)
	}
	defer rows.Close()
	wanted := make(map[string]workbook.Locale, len(workbookLocales))
	for _, locale := range workbookLocales {
		wanted[locale.Code] = locale
	}
	for rows.Next() {
		var code string
		var isDefault bool
		if scanErr := rows.Scan(&code, &isDefault); scanErr != nil {
			return fmt.Errorf("scan database locale set: %w", scanErr)
		}
		locale, exists := wanted[code]
		if !exists {
			return fmt.Errorf("workbook omits existing locale %s", code)
		}
		if locale.Default != isDefault {
			return errors.New("changing the default locale requires a dedicated migration")
		}
	}
	if errCause := rows.Err(); errCause != nil {
		return fmt.Errorf("read database locale set: %w", errCause)
	}
	return nil
}

func applyLocales(ctx context.Context, db database, locales []workbook.Locale) (int64, error) {
	var changes int64
	for _, locale := range locales {
		var fallback any
		if locale.FallbackCode != "" {
			fallback = locale.FallbackCode
		}
		tag, err := db.Exec(ctx, `
			UPDATE shop.locales SET
				native_name = $2, route_prefix = $3, fallback_code = $4,
				enabled = $5, updated_at = now()
			WHERE code = $1
			  AND (native_name, route_prefix, fallback_code, enabled)
			      IS DISTINCT FROM ($2, $3, $4, $5)
		`, locale.Code, locale.NativeName, locale.RoutePrefix, fallback, locale.Enabled)
		if err != nil {
			return 0, fmt.Errorf("update locale %s: %w", locale.Code, err)
		}
		changes += tag.RowsAffected()
		if tag.RowsAffected() != 0 {
			continue
		}
		var exists bool
		if scanErr := db.QueryRow(ctx, `SELECT EXISTS (SELECT FROM shop.locales WHERE code = $1)`, locale.Code).Scan(
			&exists,
		); scanErr != nil {
			return 0, fmt.Errorf("check locale %s: %w", locale.Code, scanErr)
		}
		if exists {
			continue
		}
		tag, err = db.Exec(ctx, `
			INSERT INTO shop.locales (
				code, native_name, route_prefix, fallback_code, enabled, is_default
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, locale.Code, locale.NativeName, locale.RoutePrefix, fallback, locale.Enabled, locale.Default)
		if err != nil {
			return 0, fmt.Errorf("insert locale %s: %w", locale.Code, err)
		}
		changes += tag.RowsAffected()
	}
	return changes, nil
}

func applyTranslations(
	ctx context.Context,
	db database,
	incoming workbook.Workbook,
	authoritativeKeys []databaseKey,
) (int64, error) {
	keys := make(map[string]databaseKey, len(authoritativeKeys))
	for _, key := range authoritativeKeys {
		keys[key.Name] = key
	}
	translations := slices.Clone(incoming.Translations)
	slices.SortFunc(translations, func(left, right workbook.Translation) int {
		leftKey, rightKey := keys[left.Key], keys[right.Key]
		if leftPriority, rightPriority := fieldPriority(
			leftKey.TargetField,
		), fieldPriority(
			rightKey.TargetField,
		); leftPriority != rightPriority {
			return leftPriority - rightPriority
		}
		if byKey := strings.Compare(left.Key, right.Key); byKey != 0 {
			return byKey
		}
		return strings.Compare(left.Locale, right.Locale)
	})
	var changes int64
	for _, translation := range translations {
		if translation.Status == workbook.StatusMissing {
			continue
		}
		key := keys[translation.Key]
		var tag pgconn.CommandTag
		var err error
		if translation.Status == workbook.StatusDelete {
			tag, err = deleteTranslation(ctx, db, key, translation.Locale)
		} else {
			tag, err = upsertTranslation(ctx, db, key, translation.Locale, translation.Translation)
		}
		if err != nil {
			return 0, fmt.Errorf("apply translation %s/%s: %w", translation.Key, translation.Locale, err)
		}
		changes += tag.RowsAffected()
	}
	return changes, nil
}

func missingRequiredCount(ctx context.Context, db database, locale string) (int64, error) {
	var missing int64
	err := db.QueryRow(ctx, missingRequiredSQL, locale).Scan(&missing)
	if err != nil {
		return 0, fmt.Errorf("check locale %s completeness: %w", locale, err)
	}
	return missing, nil
}
