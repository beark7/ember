# Ember (code name)

Open-source orchestrator for running open-weight language models on the
hardware you already own: several heterogeneous machines (Linux, macOS,
Windows) served as one OpenAI- and Anthropic-compatible endpoint, with users,
keys, quotas and a hardware-aware planner.

**Status: pre-alpha. Nothing works yet. Do not use.**

Ember does not implement inference: it orchestrates existing engines —
[llama.cpp](https://github.com/ggml-org/llama.cpp) (ggml) first — as separate
runner processes.

## Building

Requires Go 1.24+. See `AGENTS.md` for the build, test and contribution workflow.

## License

Apache-2.0 for the code (see `LICENSE`). The name and logo are trademarks.
