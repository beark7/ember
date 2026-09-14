# Ember

Read `AGENTS.md` first: it is the single source of truth for how to work in this
repository (workflow, build/test commands, hard rules, code conventions, layout).

Quick reminders:

- Work from a ticket (`docs-private/tickets/`, git-ignored, when present), on a
  branch `t<NNN>-<slug>`, one PR per ticket.
- Build and test in the cloud container: `make build test lint e2e-mock`.
- No cgo, no GPU code, no changes to security defaults, no new dependencies without
  justification, nothing from `docs-private/` into the repository. See `AGENTS.md` §4.
