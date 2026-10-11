package workbook

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	instructionsSheet   = "Instructions"
	languagesSheet      = "Languages"
	translationsSheet   = "Translations"
	metadataSheet       = "Metadata"
	deleteMarker        = "__DELETE__"
	maxWorkbookRows     = 200_000
	maxWorkbookCellLen  = 32_000
	workbookUnzipLimit  = 64 << 20
	workbookXMLLimit    = 16 << 20
	firstDataRow        = 2
	metadataFirstRow    = 7
	metadataProperties  = 3
	propertyColumns     = 2
	instructionsWidth   = 110
	translationKeyWidth = 44
	contextWidth        = 40
	localeColumnWidth   = 64
	defaultColumnWidth  = 24
)

func languageHeaders() []string {
	return []string{"code", "native_name", "route_prefix", "fallback_code", "enabled", "default"}
}

func translationHeaders() []string {
	return []string{"key", "context"}
}

func keyHeaders() []string {
	return []string{"key", "context", "required"}
}

// Encode writes a validated workbook as an XLSX document.
func Encode(writer io.Writer, item Workbook) error {
	if err := Validate(item); err != nil {
		return err
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if err := file.SetSheetName("Sheet1", instructionsSheet); err != nil {
		return fmt.Errorf("create instructions sheet: %w", err)
	}
	for _, sheet := range []string{languagesSheet, translationsSheet, metadataSheet} {
		if _, err := file.NewSheet(sheet); err != nil {
			return fmt.Errorf("create %s sheet: %w", sheet, err)
		}
	}
	if err := writeInstructions(file); err != nil {
		return err
	}
	locales := slices.Clone(item.Locales)
	slices.SortFunc(locales, func(left, right Locale) int { return strings.Compare(left.Code, right.Code) })
	if err := writeLanguages(file, locales); err != nil {
		return err
	}
	keys := slices.Clone(item.Keys)
	slices.SortFunc(keys, func(left, right Key) int { return strings.Compare(left.Name, right.Name) })
	if err := writeTranslations(file, keys, locales, item.Translations); err != nil {
		return err
	}
	if err := writeMetadata(file, item, keys); err != nil {
		return err
	}
	if err := file.SetSheetVisible(metadataSheet, false); err != nil {
		return fmt.Errorf("hide metadata sheet: %w", err)
	}
	file.SetActiveSheet(0)
	if err := file.Write(writer); err != nil {
		return fmt.Errorf("write localization workbook: %w", err)
	}
	return nil
}

// Decode reads and validates a workbook from an XLSX document.
func Decode(reader io.Reader) (Workbook, error) {
	file, err := excelize.OpenReader(reader, excelize.Options{
		UnzipSizeLimit: workbookUnzipLimit, UnzipXMLSizeLimit: workbookXMLLimit,
	})
	if err != nil {
		return Workbook{}, fmt.Errorf("open localization workbook: %w", err)
	}
	defer func() { _ = file.Close() }()
	if formulaErr := rejectFormulas(file); formulaErr != nil {
		return Workbook{}, formulaErr
	}
	for _, required := range []string{instructionsSheet, languagesSheet, translationsSheet, metadataSheet} {
		if index, indexErr := file.GetSheetIndex(required); indexErr != nil || index == -1 {
			return Workbook{}, fmt.Errorf("required sheet %s is missing", required)
		}
	}
	item, err := readMetadata(file)
	if err != nil {
		return Workbook{}, err
	}
	item.Locales, err = readLanguages(file)
	if err != nil {
		return Workbook{}, err
	}
	item.Translations, err = readTranslations(file, item.Locales)
	if err != nil {
		return Workbook{}, err
	}
	if validationErr := Validate(item); validationErr != nil {
		return Workbook{}, validationErr
	}
	return item, nil
}

func writeInstructions(file *excelize.File) error {
	rows := [][]any{
		{"Localization workbook"},
		{"Edit the Languages and Translations sheets. The Metadata sheet is generated."},
		{"Use canonical BCP-47 codes such as en-GB, de-DE, or pt-BR."},
		{"Each language is one column. Add a language in Languages, then add one column in Translations."},
		{"An empty cell is missing or unchanged. Enter __DELETE__ to remove an existing translation."},
		{"Before an import, each enabled language must hold all required translations."},
		{"The import rejects formulas, unknown keys, duplicate rows, and old revisions."},
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := file.SetSheetRow(instructionsSheet, cell, &row); err != nil {
			return fmt.Errorf("write instructions: %w", err)
		}
	}
	_ = file.SetColWidth(instructionsSheet, "A", "A", instructionsWidth)
	return nil
}

func writeLanguages(file *excelize.File, locales []Locale) error {
	if err := writeHeader(file, languagesSheet, languageHeaders()); err != nil {
		return err
	}
	for index, locale := range locales {
		row := []any{
			locale.Code,
			locale.NativeName,
			locale.RoutePrefix,
			locale.FallbackCode,
			locale.Enabled,
			locale.Default,
		}
		cell, _ := excelize.CoordinatesToCellName(1, index+firstDataRow)
		if err := file.SetSheetRow(languagesSheet, cell, &row); err != nil {
			return fmt.Errorf("write language row: %w", err)
		}
	}
	return styleTable(file, languagesSheet, "F")
}

func writeTranslations(file *excelize.File, keys []Key, locales []Locale, translations []Translation) error {
	headers := slices.Clone(translationHeaders())
	for _, locale := range locales {
		headers = append(headers, locale.Code)
	}
	if err := writeHeader(file, translationsSheet, headers); err != nil {
		return err
	}
	values := make(map[string]Translation, len(translations))
	for _, translation := range translations {
		values[translation.Key+"\x00"+translation.Locale] = translation
	}
	for index, key := range keys {
		row := []any{key.Name, key.Context}
		for _, locale := range locales {
			translation := values[key.Name+"\x00"+locale.Code]
			value := translation.Translation
			switch translation.Status {
			case StatusDelete:
				value = deleteMarker
			case StatusMissing:
				value = ""
			}
			row = append(row, value)
		}
		cell, _ := excelize.CoordinatesToCellName(1, index+firstDataRow)
		if err := file.SetSheetRow(translationsSheet, cell, &row); err != nil {
			return fmt.Errorf("write translation row: %w", err)
		}
	}
	lastColumn, _ := excelize.ColumnNumberToName(len(headers))
	if err := styleTable(file, translationsSheet, lastColumn); err != nil {
		return err
	}
	_ = file.SetColWidth(translationsSheet, "A", "A", translationKeyWidth)
	_ = file.SetColWidth(translationsSheet, "B", "B", contextWidth)
	if len(headers) > len(translationHeaders()) {
		_ = file.SetColWidth(translationsSheet, "C", lastColumn, localeColumnWidth)
	}
	return nil
}

func writeMetadata(file *excelize.File, item Workbook, keys []Key) error {
	rows := [][]any{
		{"property", "value"},
		{"workbook_version", item.Version},
		{"revision", item.Revision},
		{"exported_at", item.ExportedAt},
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := file.SetSheetRow(metadataSheet, cell, &row); err != nil {
			return fmt.Errorf("write metadata property: %w", err)
		}
	}
	header := make([]any, len(keyHeaders()))
	for index, value := range keyHeaders() {
		header[index] = value
	}
	if err := file.SetSheetRow(metadataSheet, "A6", &header); err != nil {
		return fmt.Errorf("write metadata header: %w", err)
	}
	for index, key := range keys {
		row := []any{key.Name, key.Context, key.Required}
		cell, _ := excelize.CoordinatesToCellName(1, index+metadataFirstRow)
		if err := file.SetSheetRow(metadataSheet, cell, &row); err != nil {
			return fmt.Errorf("write metadata key: %w", err)
		}
	}
	return nil
}

func writeHeader(file *excelize.File, sheet string, values []string) error {
	row := make([]any, len(values))
	for index, value := range values {
		row[index] = value
	}
	if err := file.SetSheetRow(sheet, "A1", &row); err != nil {
		return fmt.Errorf("write %s header: %w", sheet, err)
	}
	return nil
}

func styleTable(file *excelize.File, sheet, lastColumn string) error {
	style, err := file.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#F7F1E7"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#274437"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return fmt.Errorf("create workbook header style: %w", err)
	}
	if styleErr := file.SetCellStyle(sheet, "A1", lastColumn+"1", style); styleErr != nil {
		return fmt.Errorf("style %s header: %w", sheet, styleErr)
	}
	if paneErr := file.SetPanes(
		sheet,
		&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"},
	); paneErr != nil {
		return fmt.Errorf("freeze %s header: %w", sheet, paneErr)
	}
	_ = file.SetColWidth(sheet, "A", lastColumn, defaultColumnWidth)
	return nil
}

func rejectFormulas(file *excelize.File) error {
	for _, sheet := range file.GetSheetList() {
		rows, err := boundedRows(file, sheet)
		if err != nil {
			return err
		}
		for rowIndex, row := range rows {
			for columnIndex := range row {
				cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
				formula, formulaErr := file.GetCellFormula(sheet, cell)
				if formulaErr != nil {
					return fmt.Errorf("read formula at %s!%s: %w", sheet, cell, formulaErr)
				}
				if formula != "" {
					return fmt.Errorf("formula cells are not allowed: %s!%s", sheet, cell)
				}
			}
		}
	}
	return nil
}

func readMetadata(file *excelize.File) (Workbook, error) {
	rows, err := boundedRows(file, metadataSheet)
	if err != nil {
		return Workbook{}, err
	}
	if len(rows) < 6 || !equalRow(rows[0], []string{"property", "value"}) || !equalRow(rows[5], keyHeaders()) {
		return Workbook{}, errors.New("metadata sheet has an invalid structure")
	}
	properties := make(map[string]string, metadataProperties)
	for _, row := range rows[1:4] {
		if len(row) < propertyColumns {
			return Workbook{}, errors.New("metadata sheet has an incomplete property")
		}
		properties[row[0]] = row[1]
	}
	revision, err := strconv.ParseInt(properties["revision"], 10, 64)
	if err != nil {
		return Workbook{}, errors.New("metadata revision is invalid")
	}
	item := Workbook{
		Version:    properties["workbook_version"],
		Revision:   revision,
		ExportedAt: properties["exported_at"],
	}
	for rowNumber, row := range rows[6:] {
		if emptyRow(row) {
			continue
		}
		if len(row) != len(keyHeaders()) {
			return Workbook{}, fmt.Errorf("metadata row %d has %d columns", rowNumber+metadataFirstRow, len(row))
		}
		required, parseErr := strconv.ParseBool(row[2])
		if parseErr != nil {
			return Workbook{}, fmt.Errorf("metadata row %d required flag is invalid", rowNumber+metadataFirstRow)
		}
		item.Keys = append(item.Keys, Key{Name: row[0], Context: row[1], Required: required})
	}
	return item, nil
}

func readLanguages(file *excelize.File) ([]Locale, error) {
	rows, err := boundedRows(file, languagesSheet)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || !equalRow(rows[0], languageHeaders()) {
		return nil, errors.New("languages sheet has an invalid header")
	}
	locales := make([]Locale, 0, len(rows)-1)
	for rowNumber, row := range rows[1:] {
		if emptyRow(row) {
			continue
		}
		if len(row) != len(languageHeaders()) {
			return nil, fmt.Errorf("languages row %d has %d columns", rowNumber+firstDataRow, len(row))
		}
		enabled, parseErr := strconv.ParseBool(row[4])
		if parseErr != nil {
			return nil, fmt.Errorf("languages row %d enabled flag is invalid", rowNumber+firstDataRow)
		}
		isDefault, parseErr := strconv.ParseBool(row[5])
		if parseErr != nil {
			return nil, fmt.Errorf("languages row %d default flag is invalid", rowNumber+firstDataRow)
		}
		locales = append(locales, Locale{
			Code:         row[0],
			NativeName:   row[1],
			RoutePrefix:  row[2],
			FallbackCode: row[3],
			Enabled:      enabled,
			Default:      isDefault,
		})
	}
	return locales, nil
}

func readTranslations(file *excelize.File, locales []Locale) ([]Translation, error) {
	rows, err := boundedRows(file, translationsSheet)
	if err != nil {
		return nil, err
	}
	baseHeaders := translationHeaders()
	if len(rows) == 0 || len(rows[0]) < len(baseHeaders) ||
		!slices.Equal(rows[0][:len(baseHeaders)], baseHeaders) {
		return nil, errors.New("translations sheet has an invalid header")
	}
	localeSet := make(map[string]bool, len(locales))
	for _, locale := range locales {
		localeSet[locale.Code] = true
	}
	headers := rows[0][len(baseHeaders):]
	if len(headers) != len(locales) {
		return nil, errors.New("translations language columns do not match Languages")
	}
	seenLocales := make(map[string]bool, len(headers))
	for _, locale := range headers {
		if !localeSet[locale] || seenLocales[locale] {
			return nil, fmt.Errorf("translations has invalid language column %q", locale)
		}
		seenLocales[locale] = true
	}
	translations := make([]Translation, 0, (len(rows)-1)*len(headers))
	for rowNumber, row := range rows[1:] {
		if emptyRow(row) {
			continue
		}
		if len(row) < len(baseHeaders) || len(row) > len(rows[0]) {
			return nil, fmt.Errorf("translations row %d has %d columns", rowNumber+firstDataRow, len(row))
		}
		row = append(row, make([]string, len(rows[0])-len(row))...)
		for index, locale := range headers {
			value := row[index+len(baseHeaders)]
			status := StatusTranslated
			switch value {
			case "":
				status = StatusMissing
			case deleteMarker:
				status = StatusDelete
				value = ""
			}
			translations = append(translations, Translation{
				Key: row[0], Locale: locale, Translation: value, Context: row[1], Status: status,
			})
		}
	}
	return translations, nil
}

func boundedRows(file *excelize.File, sheet string) ([][]string, error) {
	rows, err := file.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("read %s sheet: %w", sheet, err)
	}
	if len(rows) > maxWorkbookRows {
		return nil, fmt.Errorf("%s sheet exceeds %d rows", sheet, maxWorkbookRows)
	}
	for rowNumber, row := range rows {
		for columnNumber, value := range row {
			if len(value) > maxWorkbookCellLen {
				return nil, fmt.Errorf("%s row %d column %d exceeds the cell limit", sheet, rowNumber+1, columnNumber+1)
			}
		}
	}
	return rows, nil
}

func equalRow(got, want []string) bool {
	return slices.Equal(got, want)
}

func emptyRow(row []string) bool {
	for _, value := range row {
		if value != "" {
			return false
		}
	}
	return true
}
