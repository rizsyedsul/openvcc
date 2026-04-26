# Contributing to Open VCC

Thanks for your interest in contributing.

## Ground rules

- Be kind. See `CODE_OF_CONDUCT.md`.
- AI coding agents working in this repo should read `instructions.md` first.
- Open an issue to discuss non-trivial changes before sending a PR. Small fixes (typos, obvious bugs, docs) can go straight to a PR.

## Dev setup

Requirements:
- Go 1.23+
- `golangci-lint`
- `docker` + `docker compose` (for the local quickstart and smoke tests)
- `tofu` 1.7+ (only if you're touching `infra/`)

Common commands:

```sh
make build          # binary at ./dist/openvcc
make test           # unit tests
make test-race      # tests with race detector + coverage
make lint           # golangci-lint
make compose-up     # local 2-cloud demo via docker compose
make compose-smoke  # boots compose, runs failover script, tears down
make tofu-check     # tofu fmt -check + tofu validate over infra/
```

## Branches and commits

- Default branch: `main`. Feature branches off `main`.
- Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`).
- Sign off your commits (`git commit -s`). The DCO is enforced by CI; we do not require a CLA.

## Pull requests

- Keep PRs focused. Smaller is better.
- Add or update tests for behavior changes.
- Update `docs/` and `CHANGELOG.md` when you add a user-visible change.
- CI must be green before review.

## Tests

- Unit tests live next to the code they test (`foo.go` ↔ `foo_test.go`).
- Integration tests that need Docker should `t.Skip()` cleanly when Docker is unavailable.
- The compose-based smoke test must keep working — it's the closest thing to an end-to-end gate.

## Releasing

Maintainers tag `v*` on `main`; the release pipeline (GoReleaser + cosign) does the rest.
