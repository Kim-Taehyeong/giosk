package store

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate는 미적용 마이그레이션을 버전 순으로 적용한다.
// 적용 이력은 schema_migrations(version) 테이블에 기록(멱등).
func Migrate(db *gorm.DB) error {
	if err := ensureMigrationsTable(db); err != nil {
		return err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}
	files, err := migrationFiles()
	if err != nil {
		return err
	}
	for _, f := range files {
		version := versionOf(f)
		if applied[version] {
			continue
		}
		if err := applyMigration(db, f, version); err != nil {
			return fmt.Errorf("migration %s: %w", f, err)
		}
	}
	return nil
}

func ensureMigrationsTable(db *gorm.DB) error {
	return db.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version VARCHAR(64) PRIMARY KEY)`,
	).Error
}

func appliedVersions(db *gorm.DB) (map[string]bool, error) {
	var versions []string
	if err := db.Raw(`SELECT version FROM schema_migrations`).Scan(&versions).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(versions))
	for _, v := range versions {
		set[v] = true
	}
	return set, nil
}

func migrationFiles() ([]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func versionOf(filename string) string {
	return strings.SplitN(filename, "_", 2)[0]
}

// applyMigration은 한 파일을 트랜잭션으로 실행하고 버전을 기록한다.
func applyMigration(db *gorm.DB, filename, version string) error {
	sqlBytes, err := migrationFS.ReadFile("migrations/" + filename)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(string(sqlBytes)).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version).Error
	})
}
