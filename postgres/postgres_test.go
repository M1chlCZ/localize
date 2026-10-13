package postgres_test

import (
	"context"
	"testing"

	"github.com/m1chlcz/localize/postgres"
	"github.com/m1chlcz/localize/workbook"
)

func TestAPIRejectsMissingPoolsAndURLs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := postgres.Export(ctx, nil); err == nil {
		t.Fatal("Export accepted a nil pool")
	}
	if _, err := postgres.Apply(ctx, nil, workbook.Workbook{}, false); err == nil {
		t.Fatal("Apply accepted a nil pool")
	}
	if _, err := postgres.Open(ctx, "  "); err == nil {
		t.Fatal("Open accepted an empty database URL")
	}
	if _, err := postgres.Open(ctx, "not a postgres url"); err == nil {
		t.Fatal("Open accepted a malformed database URL")
	}
}
