# Architecture

## What problem this solves

A "data contract" is only as good as the enforcement behind it. Writing a
JSON Schema or an Avro schema down somewhere doesn't stop a producer team
from removing a field three sprints later and breaking every downstream
consumer that was relying on it — not because anyone was careless, but
because reasoning correctly about "does this change break anything" is
genuinely non-obvious once you consider that producers and consumers don't
all deploy at the same instant. The 2026 Data Engineer hiring research
named this directly: idempotent pipelines and schema contracts are now
expected, and the differentiating skill is whether a candidate can reason
precisely about schema evolution and enforce it mechanically — in CI,
before a breaking change ever reaches production — rather than discovering
it from an incident.

## Backward vs. forward compatibility: the two questions that aren't the same question

It's tempting to think "compatible" is one property a change either has or
doesn't. It isn't — there are two independent questions, and a change can
satisfy one while failing the other:

- **Backward compatible**: can a consumer running the NEW contract's
  validation logic still correctly read data that was produced under the
  OLD contract? This is what matters when consumers upgrade first, or when
  replaying historical data against new consumer code.
- **Forward compatible**: can a consumer running the OLD contract's
  validation logic still correctly read data produced under the NEW
  contract? This is what matters during a normal rolling deploy where
  producers ship the new contract before every consumer has upgraded.

`internal/compat.Classify` computes both independently, and several of the
most common schema changes are asymmetric in a way that's easy to get
backwards if you're not careful:

| Change | Breaks backward? | Breaks forward? |
|---|---|---|
| add optional field | no | no |
| add required field | **yes** (old data lacks it) | no (old consumer ignores unknown fields) |
| remove optional field | no | no |
| remove required field | no (new consumer just doesn't look for it) | **yes** (old consumer still expects it every time) |
| optional → required | **yes** (old data may lack it) | no (old consumer only ever expected "maybe") |
| required → optional | no (old data always had it anyway) | **yes** (old consumer still expects it every time) |
| enum: add a value | no (old data only used the old subset) | **yes** (new data may use a value old consumers don't know) |
| enum: remove a value | **yes** (old data may have used it) | no (new data's narrower set is still valid under the old set) |
| type change | **yes** | **yes** |

The required↔optional pair and the enum add/remove pair are each exact
mirror images of each other — one breaks backward, the other breaks
forward, never both the same way. Getting the direction right for each is
the entire engineering content of `internal/compat/compat.go`; see the
doc comments in that file and the paired tests in `compat_test.go` (each
case is asserted independently in BOTH directions, specifically so a sign
error in one direction can't hide behind a passing assertion about the
other).

## The version-bump rule

A detected compatibility issue doesn't mean a change is forbidden — it
means it must be deliberate and visible. `internal/compat/evaluate.go`'s
rule: a breaking change (relative to the contract's declared compatibility
mode) is only allowed alongside a **major** version bump. This mirrors how
real schema registries and API versioning schemes work: minor/patch bumps
promise "nothing breaks," a major bump is the explicit signal that
consumers need to pay attention before upgrading. A contract can also try
to weaken its own declared compatibility mode (e.g. `full` → `none`) as a
way to dodge future enforcement without admitting it's doing anything
risky; `Evaluate` treats a mode weakening without a major bump as blocking
too, for the same reason — "I'll just turn off the alarm" shouldn't be a
quieter path than "I'll make the actual breaking change."

## Why a flat custom contract format, not JSON Schema or Avro directly

The engineering value in this project is the compatibility reasoning, not
schema-format parsing. A flat `name/type/required/enum` field list is
enough to express every interesting compatibility question above while
keeping `internal/contract` small enough that the parsing logic is
obviously correct, leaving the real complexity concentrated in
`internal/compat` where it belongs. A real deployment of this idea would
likely sit on top of JSON Schema or Avro and reuse their parsers; the
compatibility *rules* implemented here would carry over largely unchanged.

## Runtime enforcement, not just CI-time enforcement

A schema that's provably safe to evolve is still useless if producers can
send data that never conformed to it in the first place.
`internal/validate` checks actual JSON event data against a contract
(required fields present and non-null, types match — including the
`integer` vs `number` distinction JSON itself doesn't make, and enum
membership), and `internal/store` wires that into a shared ingestion
gateway multiple producers write through: a violating event is routed to a
dead-letter table rather than either being silently dropped or, worse,
accepted into the shared store despite not conforming to what consumers
were promised.

## Testing strategy

- `internal/semver`: version parsing and comparison, specifically the
  "major bump" predicate used to gate breaking changes (including that a
  major *downgrade* doesn't count as a bump).
- `internal/contract`: contract-file validation — duplicate fields, unknown
  types, enum declared on a non-string field, unknown compatibility mode,
  and the "compatibility defaults to full when unset" rule (the safest
  default rather than silently permissive).
- `internal/compat`: the core classification logic (the table above, each
  row as an independent test in both directions) plus `Evaluate`'s mode
  gating (each of backward-only, forward-only, full, and none correctly
  including/excluding the issues relevant to that mode) and the
  version-bump and mode-weakening rules.
- `internal/validate`: runtime event validation, including the
  integer-vs-number distinction (a whole-number float like `3.0` must
  satisfy an `integer` field, since that's how JSON numbers decode in Go;
  a fractional value like `3.5` must not) and that unknown fields are
  allowed (closed-world validation would make additive, gradual rollout
  impossible).
- `internal/store`: multi-producer ingestion — valid events accepted,
  invalid events dead-lettered (never silently dropped or silently
  accepted), and that two producers on different contract versions during
  a rolling upgrade are both tracked correctly.

## What's deliberately out of scope (v1)

- **Nested objects/arrays as contract fields.** Every field here is a
  scalar; real contract tools (Avro, Protobuf) support nested records. The
  compatibility reasoning for nested structures is a generalization of the
  same rules, not a different problem, so it was left out to keep the
  field-diffing logic easy to verify by hand.
- **Rename detection / field aliasing.** A rename looks identical to
  "remove field A, add field B" under this model and is correctly (if
  conservatively) flagged as breaking in whichever direction each half
  implies — the same behavior real schema registries without alias support
  exhibit.
- **A real external GitHub Actions composite action published for other
  repos to consume.** `.github/workflows/ci.yml` in this repo demonstrates
  the intended usage pattern (`contractgate check -old <base> -new <head>`
  as a required PR check) against its own example contracts; packaging it
  as a reusable action for other repositories is a straightforward
  follow-up, not a new engineering problem.
