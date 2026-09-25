package workbook

const Version = "1"

const (
	StatusTranslated = "translated"
	StatusMissing    = "missing"
	StatusDelete     = "delete"
)

type Locale struct {
	Code         string `json:"code"`
	NativeName   string `json:"native_name"`
	RoutePrefix  string `json:"route_prefix"`
	FallbackCode string `json:"fallback_code"`
	Enabled      bool   `json:"enabled"`
	Default      bool   `json:"default"`
}

type Key struct {
	Name     string `json:"name"`
	Context  string `json:"context"`
	Required bool   `json:"required"`
}

type Translation struct {
	Key         string `json:"key"`
	Locale      string `json:"locale"`
	Translation string `json:"translation"`
	Context     string `json:"context"`
	Status      string `json:"status"`
}

type Workbook struct {
	Version      string        `json:"version"`
	Revision     int64         `json:"revision"`
	ExportedAt   string        `json:"exported_at"`
	Locales      []Locale      `json:"locales"`
	Keys         []Key         `json:"keys"`
	Translations []Translation `json:"translations"`
}
