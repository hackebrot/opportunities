package store

import (
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/hackebrot/opportunities/db"
)

// migrationsDir is the subdirectory inside db.Migrations where goose
// finds numbered SQL files.
const migrationsDir = "migrations"

func init() {
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		panic(fmt.Sprintf("store: set goose dialect: %v", err))
	}
}
