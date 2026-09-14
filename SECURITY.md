# Security policy

Ember is pre-alpha and not yet meant for production use.

Report vulnerabilities privately through GitHub's "Report a vulnerability"
form on this repository (Security → Advisories). Please do not open public
issues for security problems. You will get an acknowledgement within 5
working days and a fix or a mitigation plan within 30 days for confirmed
issues.

Security defaults that must never change without an ADR (see ADR-008):
loopback bind, API keys required off-loopback, telemetry off, per-runner API
keys, no prompt or completion content in logs, metrics or telemetry.
