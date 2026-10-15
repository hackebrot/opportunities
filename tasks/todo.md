# M1 todo

25 tasks across 5 phases. This file is the working checklist; it is
committed so it stays in sync with the GitHub issue tracker. The detailed
task plan (`tasks/plan.md`) and underlying product spec (`SPEC.md`) are
local-only design docs not committed to this repo.

## M1/P1 — Foundation

- [x] **T01** (#1) — Repo bootstrap: `go.mod`, project tree, `cmd/opps/main.go`
      with cobra root + `opps version` + ldflags wiring, Makefile, `.gitignore`.
- [x] **T02** (#2) — CI workflow: `.github/workflows/ci.yml` (lint + gofumpt +
      unit + integration on ubuntu-latest); `.golangci.yml` with `forbidigo`
      rule for `time.Now()`; smoke testcontainers run to confirm Docker
      availability.
- [x] **T03** (#3) — Config loader (`internal/config`, `internal/log`): TOML +
      `OPPS_*` env overrides; only M1 keys are *read*, others parse as
      no-op forward compat. zap text encoder to stderr.
- [x] **T04** (#4) — i18n loader (`internal/i18n`): `embed.FS` over
      `locales/*.toml`, `i18n.T(locale, key, args...)`, empty en-US.toml
      stub. **Machinery only — CLI stays English-only.**
- [x] **T05** (#5) — Schema migration `db/migrations/00001_init.sql`: every table,
      partial unique index, composite FK on `events` (with `(opportunity_id,
      id)` unique on apps), all CHECKs. `internal/cli/db.go`: `opps db
      migrate up | down | status | redo`, `opps db reset --yes`.
- [x] **T06** (#6) — Prompt harness (`internal/prompt`): huh wrappers,
      `PickEntity`, `PickOrCreate` (inline-create branch), `--non-interactive`
      global flag.
- [x] **T24** (#32) — `opps config path` subcommand
      (`internal/cli/config.go`): print absolute path of the resolved
      config file to stdout. Minimal scope to satisfy Checkpoint A;
      `opps config get` and related forms remain in T22.

**Checkpoint A** — foundation: CI green on every PR; build, lint, test, int
all green; `opps version`, `opps db migrate up`, `opps config path` all work.

## M1/P2 — Companies + Contacts

- [x] **T07** (#7) — Companies model + store: `internal/model.Company`,
      `internal/store/companies.go` with `Create/Get/List/Update/Delete`,
      `pgx.PgError` → sentinel translation in store.
- [x] **T08** (#8) — Companies service + slug + reusable `prompt.AddCompany`:
      slug rules from spec, unit-tested edge cases. The prompt is callable
      from inline-create branches.
- [x] **T09** (#9) — Companies CLI: noun-first `opps company` parent with
      `create|list|show|update|rm` subcommands (plural alias
      `companies`); `--json` on read commands. Unit + int + e2e verification.
- [x] **T10** (#10) — Contacts: full vertical (model + store + service +
      reusable `prompt.AddContact` + CLI). Nullable company FK; default
      company prepop when called inline.

**Checkpoint B** — companies + contacts manageable end-to-end via CLI;
`AddCompany` and `AddContact` reusable as inline-create branches.

## M1/P3 — Opportunities + events engine

- [x] **T11** (#11) — Opportunity model + store + create flow:
      `Create/Get/List/Update/Delete`, `SetLatestStatus` helper.
      `service.AddOpportunity` writes opp + `added` event in one tx.
      When this lands, address #36 (extract shared Postgres
      testcontainer helper — third caller makes the refactor worth it).
- [x] **T12** (#12) — Events engine (opportunity-only kinds):
      `service.AppendEvent` for `added`/`exploring`/`archived`/`note`/
      `follow_up`/`custom`/`declined`-without-app.
      `service.RecomputeLatestStatus` implementing the 6-step rule.
      Comprehensive table-driven unit tests.
- [x] **T13** (#13) — Opportunity prompts + CLI with inline-create:
      `prompt.AddOpportunity` uses `PickOrCreate` for company *and*
      contact; all inserts in one tx. CLI: `opps opportunity`
      parent with `create | list | show | update | rm | archive | note |
      event create` subcommands (M1 kinds only). The "recruiter messaged
      me" scenario becomes one command.
- [x] **T14** (#14) — `opps opportunity contact attach` /
      `opps opportunity contact detach`: secondary path for adjusting
      links after creation. `--as <relationship>` required on detach
      (PK-driven).

**Checkpoint C** — US3, US9, US10, US13, static US14/US17 covered;
`latest_status` flips correctly; one-command inbound recruiter capture.

## M1/P4 — Applications & terminal events

- [x] **T15** (#15) — Applications store + service create + `applied` event +
      `ErrActiveExists` translation. Concurrent regression test. **No CLI
      in this task.** Also replace the raw `INSERT INTO applications` in
      `TestIntegrationInsertEventCrossOpportunityApplication`
      (`internal/store/opportunities_integration_test.go`) with a call to
      the new applications store insert.
- [x] **T16** (#16) — Application status transitions: every remaining row of
      the transition table (interview kinds, offer/counter, accepted,
      rejected/declined/withdrawn with `archive_reason_category`).
      Table-driven tests; `archived_at = events.occurred_at` mirroring.
      Also replace the raw `UPDATE applications` in
      `TestIntegrationAddApplicationReapply`
      (`internal/service/applications_integration_test.go`) with a call
      through the new rejected/declined/withdrawn service surface.
- [x] **T17** (#17) — `opps opportunity apply` + `opps opportunity event
      create` app-scoped contextual menu. Top-level `opps apply`
      registered as alias.
- [x] **T18** (#18) — `opps application follow-up [<id>] [--blocked] [--done]`
      (top-level `opps follow-up` as alias): no-flag = stamp
      `last_followed_up_at`; `--blocked` = suppress future staleness
      alerts; `--done` = clear block + restamp.
- [x] **T19** (#19) — Application prompts + CRUD CLI: noun-first
      `opps application` parent with `create` (full from-scratch with
      chained inline-create through opportunity), `list`, `show`,
      `update`, `rm`.
- [x] **T20** (#20) — Compensation & application_stages stubs: tables exist,
      store-layer CRUD scaffolded, `opps <entity> show` reads them. No CLI commands.

**Checkpoint D** — US1, US2, US8, US10 covered; all spec invariants
tested; partial-index race regression in place.

## M1/P5 — Polish & Release

- [ ] **T21** (#21) — Edge polish: `--status`/`--company`/`--archived` filters
      on `opps opportunity list` / `opps application list`; `--json`
      everywhere; full `--non-interactive` discipline (US5 contract);
      validate FK-reference flags (e.g. `--company`) before prompting.
- [ ] **T22** (#22) — `opps config get`, `opps config get <dotted.key>`,
      `opps config path`. Verify `opps version` build-flag wiring through CI.
- [ ] **T25** (#39) — Interactive `update` flow: prompt for editable current
      values when `update` runs interactively without field flags, across all
      entities (company, contact, opportunity, application). Requires a
      `prompt` helper for text input with a prefilled default. Explicit
      `--field` flags still skip prompts; `--non-interactive` no-flag update
      behaves predictably (no-op or error — decide when building).
- [ ] **T26** (#54) — Harden prompt-layer non-interactive tests: assert no
      prompts fire by installing a stub whose `Select`/`Text`/`Confirm`
      methods fail the test, instead of relying on `InterfaceFrom`'s panic
      when no `Interface` is installed.
- [ ] **T27** (#56) — Currency-aware compensation amount formatting in
      `opps opportunity show` / `opps application show`: render `base_minor`
      / `bonus_minor` as major units with per-currency separators — USD
      `95,000.00`, EUR `95.000,00` (thousands and decimal separators
      inverted). Table output only; `--json` keeps the raw integer minor
      units for machine consumers. Follow-up to T20.
- [ ] **T28** (#57) — `opps application note [<id>] [<text>]` (top-level
      `opps note` as alias): append a `note` event with `application_id`
      set, so notes attach to a specific application rather than only its
      opportunity. Picker (when no positional given) lists active
      applications. `opps opportunity note` stays for pre-application
      notes.
- [ ] **T29** (#58) — Rejection details on terminal applications: terminal
      events (`rejected`/`declined`/`withdrawn`/`accepted`) mirror the
      event's notes into `applications.archive_reason`, matching how
      `opps opportunity archive` fills `opportunities.archive_reason`. The
      interactive `opps opportunity event create` flow prompts for optional
      details after the reason category. Tighten
      `applications_archive_reason_chk` so terminal statuses other than
      `accepted` require a non-NULL category (today a NULL category passes
      the CHECK), with an integration test inserting the NULL case; add the
      neither-parent-set case to the compensations one-parent CHECK test.
- [ ] **T30** (#59) — Default `applications.applied_at` to the `applied`
      event's `occurred_at` when not supplied, per the spec's transition
      table. Interactive `opps application create` prompts for it with
      "now" as the default; `--applied-at` still overrides. Integration
      test asserts the column is set after `opps opportunity apply`.
- [ ] **T31** (#60) — e2e tier: `e2e/` package with `//go:build e2e`
      tests that build `opps`, run it as a subprocess against the local
      `opportunities_e2e` database, and assert stdout + DB state. Cover
      the exit-gate stories the plan routed to e2e: one-shot apply
      (with `applied_at` set), active app → rejected with reason category
      and details, follow-up blocked/done, re-apply after terminal,
      application notes.
- [ ] **T23** (#23) — Tag `v0.1.0`: CI green on merge commit; manual smoke run
      covering every exit-gate user story; `git tag v0.1.0`.

**Checkpoint E** — `v0.1.0` shipped: CI green, all exit-gate user stories
manually smoke-verified, schema invariants tested, Boundaries section honored
in code review.
