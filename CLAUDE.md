# Project conventions for Claude Code

`opportunities` is a local-first Go CLI (`opps`) for tracking job
opportunities, applications, contacts, and compensation. PostgreSQL-
backed, single-user, single-machine.

## Conventions

See `CONTRIBUTING.md` for git workflow, branch naming, commit message
format, issue/milestone/PR mechanics, code style, and test tiers. Follow
that document.

Don't run `goimports` or `gofumpt` manually — a Claude Code hook
formats Go files automatically on every edit.

This repo is public. No PII in committed defaults, fixtures, or example
configs — no salary numbers, real names, real emails, or addresses. Use
zero/empty/placeholder values; users supply real numbers in their local
`config.toml`. Generic structural choices (currency code, locale,
`example.com` placeholders) are fine.

CLI grammar is noun-first: `opps <entity> <verb>` — e.g.
`opps company create`, not `opps add company`. Each entity parent carries a plural
alias (`opps companies list` works). Top-level verbs are reserved for
app-level operations (`opps version`, `opps db ...`, `opps config ...`,
`opps server`, `opps dashboard`, `opps report`, `opps export`) plus a
fixed set of high-frequency state-transition shortcuts justified by
specific user stories: `opps apply`, `opps note`, `opps follow-up`. The
shortcuts are aliases over the canonical noun-first form; the canonical
form is preferred in docs and `--help` output.

## Project documents

| Document        | In git? | When to read                              |
|-----------------|---------|-------------------------------------------|
| `tasks/todo.md` | Yes     | Always — task checklist; use for "what's next?". |
| `SPEC.md`       | No      | Before substantive design or implementation work. Product spec, user stories, data model, status transitions. |
| `tasks/plan.md` | No      | When starting a specific task `T<NN>`. M1 implementation plan, per-task acceptance criteria, cross-cutting risks. |

Skip the gitignored docs for trivial edits, status checks, or non-code
discussion.

## Approval flow for git commands

The user owns all git state changes. The agent proposes the exact command
it would run; the user approves, edits, or takes over.

Example — starting a new task:

    Proposed:
    git checkout -b feat/T01-bootstrap

Example — committing finished work:

    Proposed:
    git add internal/cli/version.go cmd/opps/main.go
    git commit -m "feat(cli): Add opps version subcommand (T01)"

The `tasks/todo.md` checkbox flip is always its own follow-up commit,
never bundled into the feature commit:

    Proposed:
    git add tasks/todo.md
    git commit -m "docs(tasks): Mark T01 complete"

The user replies one of:

- approve → agent runs the proposed commands as-is
- edit (e.g. "use 'Implement' instead of 'Add'") → agent runs the edited
  version
- take over → agent stops; user runs the commands themselves

User-only (never run by the agent):

- `git rebase`
- `git reset --hard`
- `git branch -D`
- `git push`

These either rewrite history, destroy uncommitted work, or publish to
remotes — always run by the user.

Propose and wait for approval:

- `gh pr create`
- `gh issue create`

These publish state but don't destroy anything. Same approval flow as
commits.

When proposing `gh pr create`, include `--label` and `--milestone` flags
per the rules in `CONTRIBUTING.md` (`## Issues, milestones, PRs`) — set
them at creation time, not after. The `--body` must follow
`.github/PULL_REQUEST_TEMPLATE.md` (Summary bullets, Test plan with
non-CI checks, trailing `Closes #N`).

Never use `git add -A` or `git add .` — always stage by file name.
