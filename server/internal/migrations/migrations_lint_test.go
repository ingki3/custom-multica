package migrations

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const maxFrozenMigrationPrefix = 77
const frozenMigrationCount = 94
const frozenMigrationManifestSHA256 = "356d1d70793009b98d2952209123755b1c9af4bb3503dfd5b2a4d6568de5ebc5"

var migrationPrefixPattern = regexp.MustCompile(`^(\d+)_`)

func TestMigrationFilesHaveMatchingDirections(t *testing.T) {
	files := migrationFilesForLint(t, "*.sql")
	directions := make(map[string]map[string]bool)
	for _, file := range files {
		name := filepath.Base(file)
		for _, direction := range []string{"up", "down"} {
			suffix := "." + direction + ".sql"
			if strings.HasSuffix(name, suffix) {
				stem := strings.TrimSuffix(name, suffix)
				if directions[stem] == nil {
					directions[stem] = make(map[string]bool)
				}
				directions[stem][direction] = true
			}
		}
	}
	for stem, got := range directions {
		if !got["up"] || !got["down"] {
			t.Errorf("migration %s must have both .up.sql and .down.sql files", stem)
		}
	}
}

func TestFrozenMigrationManifestAndFuturePrefixes(t *testing.T) {
	files := migrationFilesForLint(t, "*.up.sql")
	frozen := make([]string, 0, frozenMigrationCount)
	futureByPrefix := make(map[int][]string)
	for _, file := range files {
		stem := strings.TrimSuffix(filepath.Base(file), ".up.sql")
		match := migrationPrefixPattern.FindStringSubmatch(stem)
		if match == nil {
			t.Fatalf("migration %s must start with a numeric prefix and underscore", stem)
		}
		prefix, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("parse migration prefix %q: %v", match[1], err)
		}
		if prefix <= maxFrozenMigrationPrefix {
			frozen = append(frozen, stem)
			continue
		}
		futureByPrefix[prefix] = append(futureByPrefix[prefix], stem)
	}
	sort.Strings(frozen)
	manifest := strings.Join(frozen, "\n") + "\n"
	gotHash := fmt.Sprintf("%x", sha256.Sum256([]byte(manifest)))
	if len(frozen) != frozenMigrationCount || gotHash != frozenMigrationManifestSHA256 {
		t.Fatalf("historical migrations 001-%03d changed: count=%d sha256=%s; never rename, remove, or add files in the frozen range", maxFrozenMigrationPrefix, len(frozen), gotHash)
	}
	for prefix, stems := range futureByPrefix {
		if len(stems) != 1 {
			sort.Strings(stems)
			t.Errorf("migration prefix %03d is reused by %v; use the next unique prefix", prefix, stems)
		}
	}
}

func migrationFilesForLint(t *testing.T, pattern string) []string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration lint test path")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(self), "..", "..", "migrations"))
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no migration files matched %s in %s", pattern, dir)
	}
	return files
}
