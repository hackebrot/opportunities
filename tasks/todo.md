# M1 todo

23 tasks across 5 phases. This file is the working checklist; it is
committed so it stays in sync with the GitHub issue tracker. The detailed
task plan (`tasks/plan.md`) and underlying product spec (`SPEC.md`) are
local-only design docs not committed to this repo.

## M1/P1 — Foundation

- [x] **T01** (#1) — Repo bootstrap: `go.mod`, project tree, `cmd/opps/main.go`
      with cobra root + `opps version` + ldflags wiring, Makefile, `.gitignore`.
- [ ] **T02** (#2) — CI workflow: `.github/workflows/ci.yml` (lint + gofumpt +
      unit + integration on ubuntu-latest); `.golangci.yml` with `forbidigo`
      rule for `time.Now()`; smoke testcontainers run to confirm Docker
      availability.
- [ ] **T03** (#3) — Config loader (`internal/config`, `internal/log`): TOML +
      `OPPS_*` env overrides; only M1 keys are *read*, others parse as
      no-op forward compat. zap text encoder to stderr.
- [ ] **T04** (#4) — i18n loader (`internal/i18n`): `embed.FS` over
      `locales/*.toml`, `i18n.T(locale, key, args...)`, empty en-US.toml
      stub. **Machinery only — CLI stays English-only.**
- [ ] **T05** (#5) — Schema migration `0001_init.up/down.sql`: every table,
      partial unique index, composite FK on `events` (with `(opportunity_id,
      id)` unique on apps), all CHECKs. `internal/cli/db.go`: `opps db
      migrate up | down | status | redo`, `opps db reset --yes`.
- [ ] **T06** (#6) — Prompt harness (`internal/prompt`): huh wrappers,
      `PickEntity`, `PickOrCreate` (inline-create branch), `--non-interactive`
      global flag.

**Checkpoint A** — foundation: CI green on every PR; build, lint, test, int
all green; `opps version`, `opps db migrate up`, `opps config path` all work.

## M1/P2 — Companies + Contacts

- [ ] **T07** (#7) — Companies model + store: `internal/model.Company`,
      `internal/store/companies.go` with `Create/Get/List/Update/Delete`,
      `pgx.PgError` → sentinel translation in store.
- [ ] **T08** (#8) — Companies service + slug + reusable `prompt.AddCompany`:
      slug rules from spec, unit-tested edge cases. The prompt is callable
      from inline-create branches.
- [ ] **T09** (#9) — Companies CLI: `opps add|list|show|update|rm company`,
      `--json`. Unit + int + e2e verification.
- [ ] **T10** (#10) — Contacts: full vertical (model + store + service +
      reusable `prompt.AddContact` + CLI). Nullable company FK; default
      company prepop when called inline.

**Checkpoint B** — companies + contacts manageable end-to-end via CLI;
`AddCompany` and `AddContact` reusable as inline-create branches.

## M1/P3 — Opportunities + events engine

- [ ] **T11** (#11) — Opportunity model + store + create flow:
      `Create/Get/List/Update/Delete`, `SetLatestStatus` helper.
      `service.AddOpportunity` writes opp + `added` event in one tx.
- [ ] **T12** (#12) — Events engine (opportunity-only kinds):
      `service.AppendEvent` for `added`/`exploring`/`archived`/`note`/
      `follow_up`/`custom`/`declined`-without-app.
      `service.RecomputeLatestStatus` implementing the 6-step rule.
      Comprehensive table-driven unit tests.
- [ ] **T13** (#13) — Opportunity prompts + CLI with inline-create:
      `prompt.AddOpportunity` uses `PickOrCreate` for company *and*
      contact; all inserts in one tx. CLI: `add | list | show | update |
      rm | archive | note | event add` (M1 kinds only). The
      "recruiter messaged me" scenario becomes one command.
- [ ] **T14** (#14) — `opps attach contact` / `opps detach contact`: secondary
      path for adjusting links after creation. `--as <relationship>`
      required on detach (PK-driven).

**Checkpoint C** — US3, US9, US10, US13, static US14/US17 covered;
`latest_status` flips correctly; one-command inbound recruiter capture.

## M1/P4 — Applications & terminal events

- [ ] **T15** (#15) — Applications store + service create + `applied` event +
      `ErrActiveExists` translation. Concurrent regression test. **No CLI
      in this task.**
- [ ] **T16** (#16) — Application status transitions: every remaining row of
      the transition table (interview kinds, offer/counter, accepted,
      rejected/declined/withdrawn with `archive_reason_category`).
      Table-driven tests; `archived_at = events.occurred_at` mirroring.
- [ ] **T17** (#17) — `opps apply` shortcut + `opps event add` app-scoped
      contextual menu.
- [ ] **T18** (#18) — `opps follow-up [<application-id>] [--blocked] [--done]`:
      no-flag = stamp `last_followed_up_at`; `--blocked` = suppress
      future staleness alerts; `--done` = clear block + restamp.
- [ ] **T19** (#19) — Application prompts + CRUD CLI: `opps add application`
      (full from-scratch with chained inline-create through opportunity),
      `list`, `show`, `update`, `rm`.
- [ ] **T20** (#20) — Compensation & application_stages stubs: tables exist,
      store-layer CRUD scaffolded, `opps show` reads them. No CLI commands.

**Checkpoint D** — US1, US2, US8, US10 covered; all spec invariants
tested; partial-index race regression in place.

## M1/P5 — Polish & Release

- [ ] **T21** (#21) — Edge polish: `--status`/`--company`/`--archived` filters
      on `list opportunities`/`list applications`; `--json` everywhere;
      full `--non-interactive` discipline (US5 contract).
- [ ] **T22** (#22) — `opps config get`, `opps config get <dotted.key>`,
      `opps config path`. Verify `opps version` build-flag wiring through CI.
- [ ] **T23** (#23) — Tag `v0.1.0`: CI green on merge commit; manual smoke run
      covering every exit-gate user story; `git tag v0.1.0`.

**Checkpoint E** — `v0.1.0` shipped: CI green, all exit-gate user stories
manually smoke-verified, schema invariants tested, Boundaries section honored
in code review.
