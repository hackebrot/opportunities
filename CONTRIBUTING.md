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
  with `Closes #N` so the merge auto-closes it.
- `tasks/todo.md` is the canonical local task list. The PR for a planned
  task flips that task's checkbox from `[ ]` to `[x]` in the same merge.

## Code style

| Tool             | Purpose                            |
|------------------|------------------------------------|
| `gofumpt`        | Formatting. Stricter than `gofmt`. |
| `golangci-lint`  | Linting.                           |

Both run in CI.

## Tests

| Tier        | Command                                              | DB                                | Runs in CI |
|-------------|------------------------------------------------------|-----------------------------------|------------|
| Unit        | `go test ./...`                                      | None — fakes/in-memory only.      | Yes        |
| Integration | `go test -tags=integration -count=1 ./...`           | testcontainers Postgres 16.       | Yes        |
| e2e         | `go test -tags=e2e -count=1 ./...`                   | Local Homebrew Postgres.          | No         |

The `Makefile` mirrors these as `make test`, `make int`, `make e2e`.
