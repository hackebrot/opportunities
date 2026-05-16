// Package db exposes the embedded SQL migrations consumed by
// internal/store. Keeping the embed.FS here lets the migrations live
// at repo-root db/migrations (Go's //go:embed cannot cross parent
// directories from internal/store).
package db

import "embed"

// Migrations is the embedded filesystem rooted at db/, containing
// migrations/0001_init.up.sql / .down.sql and any future versions.
// Consumers pass it to goose.SetBaseFS and address the directory as
// "migrations".
//
//go:embed migrations/*.sql
var Migrations embed.FS
