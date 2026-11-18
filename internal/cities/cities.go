// Package cities searches the worldwide city list (~169k GeoNames entries)
// stored as SQLite. The database ships gzipped inside the binary and is
// extracted once, on the first search, to the state folder; queries then run
// inside SQLite, so the list is never held in memory.
package cities

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/uzuki-P/pray-time-mygo/internal/prayer"
)

//go:embed cities.db.gz
var compressed []byte

// DefaultLimit is how many matches a search returns.
const DefaultLimit = 50

// Catalog opens the extracted database lazily and keeps it open.
type Catalog struct {
	dir  string
	mu   sync.Mutex
	db   *sqlite.DB
	open error
}

// New keeps the database in dir.
func New(dir string) *Catalog { return &Catalog{dir: dir} }

func (c *Catalog) path() string { return filepath.Join(c.dir, "cities.db") }

func (c *Catalog) connect(ctx context.Context) (*sqlite.DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != nil {
		return c.db, nil
	}
	if err := c.extract(); err != nil {
		return nil, err
	}
	db, err := sqlite.Open(ctx, c.path(), sqlite.OpenOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	c.db = db
	return db, nil
}

// extract writes the database once; a non-empty file is kept. The Kotlin
// app's legacy TSV list is removed, as it did.
func (c *Catalog) extract() error {
	defer os.Remove(filepath.Join(c.dir, "cities_world.tsv"))
	if fi, err := os.Stat(c.path()); err == nil && fi.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	tmp := c.path() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, zr)
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("extract cities: %v %v", copyErr, closeErr)
	}
	return os.Rename(tmp, c.path())
}

// Close closes the database if it was opened.
func (c *Catalog) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db != nil {
		c.db.Close()
		c.db = nil
	}
}

func escapeLike(v string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v)
}

// Search finds regions whose name, admin area or country contains query,
// names that start with it first.
func (c *Catalog) Search(ctx context.Context, query string, limit int) ([]prayer.Region, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	db, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT name, admin, country, latitude, longitude, timezone
		FROM cities
		WHERE lower(name) LIKE ?1 ESCAPE '\' OR lower(admin) LIKE ?1 ESCAPE '\' OR lower(country) LIKE ?1 ESCAPE '\'
		ORDER BY (lower(name) LIKE ?2 ESCAPE '\') DESC, lower(name)
		LIMIT ?3`,
		"%"+escapeLike(q)+"%", escapeLike(q)+"%", int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]prayer.Region, 0, len(rows.Values))
	for _, r := range rows.Values {
		if len(r) < 6 {
			continue
		}
		out = append(out, prayer.Region{
			Name: str(r[0]), AdminName: str(r[1]), Country: str(r[2]),
			Latitude: num(r[3]), Longitude: num(r[4]), TimezoneID: str(r[5]),
		})
	}
	return out, nil
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}

func num(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	return 0
}
