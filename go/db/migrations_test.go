package db

import (
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// migrationName is dbmate's file name: a fourteen-digit version, an underscore,
// a description, and the .sql extension.
var migrationName = regexp.MustCompile(`^(\d{14})_.+\.sql$`)

// TestMigrationVersionsAreUnique guards the one way a migration can fail
// silently: dbmate records an applied migration by its version alone, so two
// files that share a version are one migration to it, and when the first has
// been applied the second is skipped without a word on `make db-up`. Branches
// that both guess a round time for the same day collide this way (the
// jurisdiction_citations migration of issue #216 did, against
// hun-vroom-diffvols), which is why CLAUDE.md asks for the version to come from
// the clock. The test also requires the version to be a real timestamp, so
// that a typo cannot sort a migration into the past.
func TestMigrationVersionsAreUnique(t *testing.T) {
	migrations, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 {
		t.Fatal("found no migrations to check")
	}

	seen := make(map[string]string, len(migrations))
	for _, path := range migrations {
		name := filepath.Base(path)
		m := migrationName.FindStringSubmatch(name)
		if m == nil {
			t.Errorf("%s is not named YYYYMMDDHHMMSS_description.sql", name)
			continue
		}
		version := m[1]
		if _, err := time.Parse("20060102150405", version); err != nil {
			t.Errorf("%s: version %s is not a timestamp: %v", name, version, err)
		}
		if other, ok := seen[version]; ok {
			t.Errorf("%s and %s share version %s; dbmate would apply only one of them", other, name, version)
		}
		seen[version] = name
	}
}
