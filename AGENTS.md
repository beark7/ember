# Ember — instructions for coding agents

Ember is a Go orchestrator for local LLM inference, positioned as the LLM server
for small organizations on the hardware they already own (see `docs-private/positioning.md` when
available): several heterogeneous machines seen as one endpoint with users, keys,
quotas and accounting, plus a planner that sizes the fleet. It does not implement
inference kernels: it downloads prebuilt engines (llama.cpp today, ds4 later),
starts them as separate "runner" processes and governs them (planning, routing,
scheduling, auth, accounting, API compatibility with OpenAI and Anthropic clients).

Code, comments and identifiers are in English. Tickets, ADRs and design notes may
be in Italian. This file is the entry point; read it fully before the first change.

## 1. Before touching code

1. Design notes, ADRs and tickets live in `docs-private/` — a git-ignored folder
   that is **not part of the repository** (the author keeps it in the checkout).
   When it is present: read `docs-private/positioning.md` once, then
   `docs-private/design/README.md`, then the design document of the component you
   are changing (`docs-private/design/<component>.md`). Load only what you need.
   When it is absent, the ticket text you were given is the specification; ask
   for the design note if the ticket refers to one.
2. Read the ADRs referenced by that document (`docs-private/adr/`). If your change
   contradicts an ADR, stop and propose a new ADR in the PR instead of implementing.
3. Read the ticket (`docs-private/tickets/T-NNN-*.md`) you were given. Work on that
   ticket only. If you discover a bug or a gap outside its scope, open an issue (or
   note it in the PR description under "Out of scope"), do not fix it in the same PR.
4. Never commit anything from `docs-private/`, never copy its content into the
   repository, and do not quote positioning or business notes in code, commits or
   PR descriptions. Design decisions that the code needs are summarized in
   package `doc.go` comments and in this file.
5. Look for existing tests in the package: your change must keep them passing or
   update them with a stated reason.

## 2. How work happens (Cowork workflow)

- The canonical repository is on GitHub. Build and test happen in the cloud
  container (Go 1.24, Node 22, golangci-lint are available there). The local VM on
  the author's machine has no Go toolchain and no network: do not build there.
- One ticket = one branch = one PR. Branch name: `t<NNN>-<slug>` (e.g. `t007-openai-chat`).
- Start every non-trivial ticket by posting a plan of at most 10 lines in the PR
  description (files to touch, interfaces involved, tests to add). Then implement.
- Keep PRs under ~500 changed lines excluding generated files and testdata. If the
  ticket does not fit, split it and say so.
- Commit messages follow Conventional Commits (`feat(server): …`, `fix(planner): …`,
  `test(gguf): …`, `docs(adr): …`).
- The PR description must contain: ticket id, summary, "How I verified it" (exact
  commands and their result), "Out of scope" notes, and a checklist:
  `[ ] tests added  [ ] lint clean  [ ] design note in docs-private updated if behavior changed`.

## 3. Build, lint, test

```
make build        # go build ./...
make test         # go test ./... -race -count=1
make lint         # golangci-lint run
make ui           # web/ build (needs Node); backend builds without it via -tags noui
make testdata     # downloads the tiny test GGUFs into testdata/models/ (git-ignored, cached)
make e2e-mock     # end-to-end against the mock engine
make e2e-tiny     # end-to-end with testdata/models/stories260K.gguf on CPU (downloads once)
```

All four of `build`, `test`, `lint`, `e2e-mock` must pass before opening a PR.
`e2e-tiny` must pass for changes in `internal/engine`, `internal/gguf`,
`internal/catalog`, `cmd/`.

## 4. Hard rules

- **No cgo** in `cmd/emberd`, `cmd/ember` and everything under `internal/`. The only
  cgo-allowed binary is `cmd/ember-tray` (year 2).
- **Never write GPU code** (Vulkan/CUDA/Metal/ROCm). The engine binary is the only
  thing that touches the GPU, including for probing (`--list-devices`) and benchmarks.
- **Never change security/network defaults**: bind `127.0.0.1:11500`, API keys
  required for non-localhost listeners, telemetry off, per-runner API key.
- **No new dependencies** without a one-paragraph justification in the PR and a
  license check (Apache-2.0/MIT/BSD compatible). Prefer the standard library.
- **No prompt or completion content** in logs, metrics, telemetry or error messages.
- Interfaces in `internal/engine`, `internal/server`, `internal/config`,
  `internal/store` are contracts: changing them requires an ADR or an updated design
  note in `docs-private/` and an explicit mention in the PR.
- Planner estimates change only with an updated golden file **and** a comment
  explaining the new number (ideally with a real measurement).
- Never commit binaries, models or `web/dist`. Testdata GGUFs are downloaded by
  `make` into a cached, git-ignored location.
- **Never commit secrets**: no `.env`, keys, certificates, tokens or API keys in
  code, tests, fixtures or docs (use `.env.example` and placeholders like
  `emb_test_…`). CI runs gitleaks on every PR; a hit blocks the merge.
- Do not disable tests, do not add `//nolint` without a reason on the same line, do
  not weaken `-race`.
- Do not change the default port, the config schema version or the DB migration
  numbering of already-merged migrations.

## 5. Code conventions

- Small packages, minimal exported surface, no `util`/`common`/`helpers` packages.
- Every package has `doc.go` stating its responsibility in ≤ 5 lines.
- Errors: `fmt.Errorf("…: %w", err)`; sentinel errors (`var ErrX = errors.New`) for
  cases callers handle. No panics outside `main` and tests.
- `context.Context` is the first parameter of anything that does I/O or waits.
- No mutable package-level state. Dependencies are passed explicitly (constructor
  functions with an options struct when > 3 params).
- Logging with `log/slog`, JSON handler; attach `request_id`, `user_id`, `model_id`,
  `runner_id` when known.
- Tests: table-driven; golden files under `testdata/`; use `testing/fstest`,
  `httptest`, and the mock engine rather than real processes wherever possible.
  Every observable behavior gets a test. A test that only checks "no error" is not
  a test.
- Platform-specific code lives in `_linux.go`, `_darwin.go`, `_windows.go` files
  behind an interface with a fixture-driven implementation for tests.

## 6. Where things are

```
cmd/emberd            daemon             internal/probe     hardware profile (parsers + engine-based bench)
cmd/ember             CLI                internal/planner   memory/speed estimates, placement, calibration
cmd/ember-tray        tray (year 2; cgo ok) internal/catalog   curated models, HF sources, remote GGUF header
internal/gguf         GGUF header parser internal/engine    Engine/Runner interfaces, RunnerManager, llamacpp, mock
internal/server       HTTP: openai/, anthropic/, mgmt/, middleware
internal/auth         API keys, admin session, roles       internal/sched      per-runner queues
internal/accounting   usage records, Prometheus metrics    internal/store      SQLite, migrations, repositories
internal/config       YAML loading, schema, env overrides  internal/fleet      node/controller, routing, quotas (phase 5)
internal/update       update check (later: apply)          internal/telemetry  opt-in reports
internal/planner/fleet fleet sizing (phase 6)
web/                  React SPA          catalog/           curated YAML       engines/   engine manifests
testdata/             hardware fixtures (raw + parsed), GGUF cache, golden files
docs-private/         design notes, ADRs, tickets, roadmap (git-ignored, not in the repository)
```

## 7. Working style

Prefer small, reversible patches. If a requirement is ambiguous, write the question
in the PR and pick the most conservative interpretation. When you find that a
design note is wrong or incomplete, fix it in `docs-private/` and say so in the PR.
When you cannot verify something (needs a GPU, macOS or Windows), say exactly what
the author must run and what output to expect, in the "How I verified it" section.
