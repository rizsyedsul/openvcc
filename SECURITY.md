# Security Policy

## Supported versions

Open VCC is pre-1.0. Only the latest tagged release receives security fixes.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

Use GitHub's private vulnerability reporting:
- Go to the repository → **Security** tab → **Report a vulnerability**.

Include reproduction steps, affected versions, and impact. We aim to acknowledge reports within 72 hours.

## Scope

In scope:
- The `openvcc` binary and its packages under `internal/` and `pkg/`.
- The OpenTofu modules under `infra/modules/`.
- Reference apps under `examples/` (when used as documented).

Out of scope:
- Third-party services the user chooses to put in front of (or behind) Open VCC.
- Issues that require an attacker who already has the engine's admin bearer token or shell access to the host.
