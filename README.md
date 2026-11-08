# contractgate

A CI gate for data contracts: given two versions of a contract, it tells
you precisely which compatibility direction(s) a change breaks — backward,
forward, or both — and fails the build unless a breaking change comes with
an explicit major version bump. Also enforces the same contract at
ingestion time against a shared multi-producer data store, so a schema
that's provably safe to evolve doesn't get undermined by data that never
conformed to it in the first place.

```
contracts/orders/v1.yaml  ──┐
                             ├─▶ contractgate check ─▶ backward/forward issues ─▶ pass/BLOCKED
contracts/orders/v2.yaml  ──┘         (exit 0 / exit 1 — wire into any CI)

producer A ──┐
producer B ──┼─▶ validate against current contract ─▶ shared store (SQLite)
producer C ──┘                                    └─▶ dead-letter table (rejected events)
```

## Why this project

The research behind this portfolio's Data Engineer projects kept pointing
at data contracts as now-expected infrastructure, and the real
differentiator as whether someone can reason *correctly* about schema
evolution — not just wire up a schema file. Backward and forward
compatibility are two genuinely different questions (see
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full table of which
common changes break which direction — several are exact mirror images of
each other and easy to get backwards), and this project's test suite nails
down each one independently in both directions specifically so a sign error
can't hide.

## Quickstart

```bash
make build
make test        # 35+ tests across semver/contract/compat/validate/store
make demo        # CI-gate decisions + a multi-producer ingestion scenario
```

```bash
./bin/contractgate check -old contracts/orders/v1.yaml -new contracts/orders/v2-compatible.yaml
# exit 0: adds an optional field, nothing breaks

./bin/contractgate check -old contracts/orders/v1.yaml -new contracts/orders/v2-breaking.yaml
# exit 1: removes a required field without a major version bump — BLOCKED

./bin/contractgate validate -contract contracts/orders/v1.yaml -data some_events.json
```

## Using this as a real CI gate

`.github/workflows/ci.yml` in this repo runs `contractgate check` against
its own example contracts on every push/PR — proving the gate actually
blocks a real breaking change in a real CI environment, not just in a unit
test (see the `contract-gate-self-check` job). The intended usage in a
real producer team's repo: run `contractgate check -old <(git show
origin/main:contracts/orders.yaml) -new contracts/orders.yaml` as a
required PR check whenever a contract file changes, so a breaking schema
change can never merge without its author consciously bumping the major
version.

## What's actually tested, concretely

- **Compatibility classification**: every row of the backward/forward
  table in docs/ARCHITECTURE.md, each direction asserted independently —
  adding/removing required and optional fields, required↔optional
  transitions, enum narrowing/expansion, restricting/relaxing a
  previously-unrestricted field, and type changes.
- **The CI-gate decision**: breaking changes blocked without a major bump
  and allowed with one; non-breaking changes never blocked regardless of
  version; each compatibility mode (backward-only, forward-only, full,
  none) correctly including or excluding the issues relevant to it; and a
  contract quietly weakening its own compatibility mode is itself treated
  as a blockable change.
- **Runtime validation**: required/optional/type/enum checks against actual
  JSON data, including that a JSON number like `3.0` satisfies an
  `integer` field while `3.5` does not (JSON doesn't distinguish int from
  float at the wire level — Go decodes both into `float64`).
- **Multi-producer ingestion**: valid events accepted into a shared SQLite
  store, invalid events routed to a dead-letter table (never silently
  dropped or silently accepted), and multiple contract versions correctly
  tracked as legitimately in-flight at once during a rolling upgrade.

## Project layout

```
internal/contract/   contract format + loading/validation
internal/semver/      minimal semver parsing/comparison
internal/compat/       the compatibility classification + CI-gate decision engine
internal/validate/     runtime event validation against a contract
internal/store/        SQLite-backed multi-producer ingestion gateway
cmd/contractgate/       CLI: check, validate, demo
contracts/orders/       example contract versions (compatible + breaking)
.github/workflows/ci.yml  real CI: tests + a self-check that the gate blocks a real breaking change
docs/ARCHITECTURE.md    design rationale, including the full compatibility table
```

## What's deliberately out of scope (v1)

See docs/ARCHITECTURE.md — no nested objects/arrays as fields, no
rename/alias detection, and the GitHub Actions workflow demonstrates the
intended usage pattern against this repo's own contracts rather than being
packaged as a reusable action for other repositories.
