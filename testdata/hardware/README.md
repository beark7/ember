# Hardware fixtures

- `raw/<machine>/` — raw command outputs captured with
  `scripts/capture-hardware.sh <machine>` (Linux/macOS) or
  `scripts/capture-hardware.ps1 -Machine <machine>` (Windows). One file per
  command, `<name>.missing` when the tool was absent, `meta.json` with OS, arch
  and free-text notes. Host name, user name and home directory are scrubbed by
  the scripts; **review the files before committing anyway**.
- `profiles/<machine>.json` — golden `HardwareProfile` produced by the probe
  parsers from the raw folder (phase 3).

Machine slug convention: `<owner>-<cpu-or-model>-<ram>[-<gpu>]`, lowercase,
e.g. `orso-m2-24gb`, `orso-pc-i7-32gb-rtx4070`, `tester-i5-8250u-16gb`.
