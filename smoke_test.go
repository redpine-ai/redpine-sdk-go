package redpine

import (
	"context"
	"os"
	"testing"
)

func TestSmoke(t *testing.T) {
	if os.Getenv("SMOKE") != "1" {
		t.Skip("set SMOKE=1 to run live")
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.Quota(ctx); err != nil {
		t.Fatal(err)
	}
	cols, err := c.Collections(ctx)
	if err != nil || cols.Count != len(cols.Collections) {
		t.Fatalf("collections: %v", err)
	}
	if cols.Count > 0 {
		if _, err := c.Search(ctx, "test", SearchOptions{Collection: cols.Collections[0].Name, Limit: 1}); err != nil {
			t.Fatal(err)
		}
	}
}
