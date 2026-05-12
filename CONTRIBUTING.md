# Contributing

This is a personal project; contributions are welcome.

## Git workflow

- `main` is protected. Never commit directly.
- One branch per task. One PR per task. Merge with `--no-ff`; delete
  branches after merge.
- Multiple commits per branch are encouraged when there are logical
  boundaries.

### Branch naming

| Branch shape                       | When                  | Example                          |
|------------------------------------|-----------------------|----------------------------------|
| `<type>/T<NN>-<short-name>`        | Planned task          | `feat/T01-bootstrap`             |
| `<type>/<short-name>`              | Unplanned setup / fix | `chore/project-bootstrapping`    |

Allowed `<type>` prefixes:

| Type       | When to use                                                  |
|------------|--------------------------------------------------------------|
| `feat`     | New user-facing capability or significant internal feature.  |
| `fix`      | Bug fix.                                                     |
| `chore`    | Maintenance, tooling, configuration. No behavior change.     |
| `docs`     | Documentation-only.                                          |
| `refactor` | Code restructuring without behavior change.                  |
| `test`     | Tests added or refactored, no production-code change.        |
| `ci`       | CI configuration / workflow changes.                         |
| `build`    | Build system, dependencies, release tooling.                 |

## Commit messages

Conventional Commits format:

```
type(scope?): Subject (T<NN>)

[optional body explaining why]
```

- `type`: from the table above.
- `scope`: package or area touched (optional).
- Subject: capital first letter, imperative, no trailing period, ≤72 chars.
- `(T<NN>)` at the end of the subject for planned-task commits.
- Body explains *why*; the diff shows *what*.

| Example commit                                                          | Notes                          |
|-------------------------------------------------------------------------|--------------------------------|
| `feat(cli): Add opps version subcommand (T01)`                          | Planned task, scoped to `cli`. |
| `feat(migrations): Initial schema with all v1 tables (T05)`             | Planned task, scope = package. |
| `ci: Add testcontainers smoke run to confirm Docker (T02)`              | No scope needed.               |
| `fix(store): Translate UniqueViolation to ErrActiveExists (T15)`        | Bug fix during planned task.   |
| `chore: Add CONTRIBUTING and CLAUDE docs`                               | Unplanned chore, no task ID.   |

## Issues, milestones, PRs

- Tasks are tracked as GitHub issues, grouped into per-phase milestones
  (`M1/P1 — Foundation`, `M1/P2 — Companies + Contacts`, etc.).
- PRs file against the matching milestone and reference the issue
  with `Closes #N` in the PR description (not in commit messages) so
  the merge auto-closes it.
- `tasks/todo.md` is the canonical local task list. The PR for a planned
  task flips that task's checkbox from `[ ]` to `[x]` in the same merge.

### PR labels

Every PR carries:

- **One type label** matching the branch's commit-type prefix:

  | Branch prefix | Label           |
  |---------------|-----------------|
  | `feat`        | `feature`       |
  | `fix`         | `bug`           |
  | `docs`        | `documentation` |
  | `refactor`    | `refactor`      |
  | `test`        | `test`          |
  | `ci`          | `ci`            |

  `chore` and `build` PRs go unlabeled on type unless a quality label
  fits (`code quality`, `test`, `ci`).

- **One or more `area:*` labels** — one per package the diff touches
  (`area:cli`, `area:prompt`, `area:service`, `area:store`, `area:config`,
  `area:i18n`, `area:migrations`).
- **Quality labels** when the PR is primarily about that concern:
  `code quality` (lint/format), `ci` (workflow), `test` (test infra or
  coverage).
- **`dependencies`** is applied automatically by Dependabot; don't add
  it manually.

## Code style

| Tool             | Purpose                              |
|------------------|--------------------------------------|
| `goimports`      | Import management and formatting.    |
| `gofumpt`        | Formatting. Stricter than `gofmt`.   |
| `golangci-lint`  | Linting.                             |

`goimports` and `gofumpt` are pinned as Go module tools and invoked via
`go tool <name>`. `golangci-lint` is installed separately per
[upstream guidance](https://golangci-lint.run/docs/welcome/install/local/).
All three will run in CI once T02 lands.

## Tests

| Tier        | Command                                                                 | DB                                | CI (once T02 lands) |
|-------------|-------------------------------------------------------------------------|-----------------------------------|---------------------|
| Unit        | `go test ./...`                                                         | None — fakes/in-memory only.      | Yes                 |
| Integration | `go test -tags=integration -run '^TestIntegration' -count=1 ./...`      | testcontainers Postgres 16.       | Yes                 |
| e2e         | `go test -tags=e2e -run '^TestE2E' -count=1 ./...`                      | Local Homebrew Postgres.          | No                  |

The `Makefile` mirrors these as `make test`, `make int`, `make e2e`.

### Naming integration and e2e tests

Go has no built-in way to tag tests with arbitrary labels, so we use
two layered conventions to keep tiers separate:

1. **Build tag on the file.** Integration test files start with
   `//go:build integration`; e2e files with `//go:build e2e`. This
   excludes them from `go test ./...` at compile time.
2. **Function-name prefix.** Integration test functions are named
   `TestIntegration<...>`; e2e functions are named `TestE2E<...>`. CI
   selects them with `-run '^TestIntegration'` / `-run '^TestE2E'` so
   the integration and e2e steps don't re-run the unit suite.

Both are required: the build tag keeps the test out of the default
build; the name prefix lets the tag's job run only the tests it owns.
