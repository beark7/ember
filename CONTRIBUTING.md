# Contributing

Ember is pre-alpha. Before opening a pull request:

1. Read `AGENTS.md` (workflow, build/test commands, hard rules, conventions).
   It applies to humans as much as to coding agents.
2. One issue per pull request; branches are named `t<NNN>-<slug>` for the
   maintainers' tickets or `issue-<N>-<slug>` for GitHub issues.
3. `make build test lint` must pass; touched behavior needs tests.
4. Conventional Commits (`feat(server): …`, `fix(planner): …`).
5. Changes to the interfaces in `internal/engine`, `internal/server`,
   `internal/config`, `internal/store` need an ADR or an updated design note in
   the same PR.

Hardware fixtures are the most valuable contribution: run
`scripts/capture-hardware.sh <machine-slug>` (or `.ps1` on Windows) and open a
PR with the resulting `testdata/hardware/raw/<machine-slug>/` folder after
checking it contains no personal data.

By contributing you agree that your contributions are licensed under the
Apache License 2.0 (see `LICENSE`).
