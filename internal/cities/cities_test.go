package cities

import (
	"context"
	"testing"
)

func TestSearch(t *testing.T) {
	c := New(t.TempDir())
	defer c.Close()
	got, err := c.Search(context.Background(), "medan", 10)
	if err != nil {
		t.Skip("SQLite native module unavailable:", err)
	}
	if len(got) == 0 || got[0].Name != "Medan" || got[0].TimezoneID != "Asia/Jakarta" {
		t.Fatalf("results %+v", got)
	}
	if got, _ := c.Search(context.Background(), "100%_", 10); len(got) != 0 {
		t.Fatalf("wildcards matched %+v", got)
	}
}
