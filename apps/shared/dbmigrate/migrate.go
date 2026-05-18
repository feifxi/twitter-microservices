package dbmigrate

import (
	"log/slog"
	"net/url"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Source: MIGRATIONS_PATH env (default "file:///migrations").
// Table: MIGRATIONS_TABLE env (default "schema_migrations").
// x-migrations-table is appended internally — dbURL must be a clean Postgres DSN.
func Run(dbURL string, log *slog.Logger) {
	src := os.Getenv("MIGRATIONS_PATH")
	if src == "" {
		src = "file:///migrations"
	}
	if err := RunSource(src, migrateURL(dbURL)); err != nil {
		log.Error("run migrations", slog.Any("err", err))
		os.Exit(1)
	}
	log.Info("migrations up to date")
}

// RunSource is the test-safe variant (returns error instead of os.Exit).
// search_path is stripped — the schema doesn't exist yet, so postgres rejects the connection if it's present.
func RunSource(source, dbURL string) error {
	m, err := migrate.New(source, stripSearchPath(dbURL))
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

// x-migrations-table is kept out of DATABASE_URL so pgxpool never sees it.
func migrateURL(dbURL string) string {
	table := os.Getenv("MIGRATIONS_TABLE")
	if table == "" {
		return dbURL
	}
	u, err := url.Parse(dbURL)
	if err != nil {
		return dbURL
	}
	q := u.Query()
	q.Set("x-migrations-table", table)
	u.RawQuery = q.Encode()
	return u.String()
}

func stripSearchPath(dbURL string) string {
	u, err := url.Parse(dbURL)
	if err != nil {
		return dbURL
	}
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String()
}
