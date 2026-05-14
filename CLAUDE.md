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
    git add internal/cli/version.go cmd/opps/main.go tasks/todo.md
    git commit -m "feat(cli): Add opps version subcommand (T01)"

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
them at creation time, not after.

Never use `git add -A` or `git add .` — always stage by file name.
