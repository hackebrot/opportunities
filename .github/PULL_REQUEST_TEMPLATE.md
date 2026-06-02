<!--
Before opening: set the matching milestone and the labels required by
CONTRIBUTING.md (type label by branch prefix — `chore`/`build` may go
unlabeled; `area:*` per package touched OR `project` for repo-level
changes; plus any quality labels).
If a linked issue exists, keep the trailing `Closes #N` so the merge
auto-closes it; otherwise remove that line.
-->

## Summary

<!-- A few bullets: lead with an imperative verb, reference code with backticks, follow with what/why. -->

- Add `<symbol>` — one sentence on what it does and why.
- Refactor `<area>` to `<change>`; note the motivation if non-obvious.

## Test plan

CI runs lint, `go mod tidy`, unit tests, and integration tests on every PR.

Additional checks not covered by CI:

- [ ] `make e2e` — for whole-binary behavior that integration tests can't reach.
- [ ] Manual verification — note commands and observed result.

<!-- Remove the line below if there is no linked issue. -->
Closes #N
