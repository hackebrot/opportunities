<!--
Before opening: set one type label (feature/bug/refactor/documentation/test/ci),
one or more area:* labels, and the matching milestone. See CONTRIBUTING.md
for the full rules. Reference the issue with `Closes #N` so the merge
auto-closes it.
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
Closes #
