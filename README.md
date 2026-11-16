# contractgate

A CI gate for data contracts, written in Go. Give it two versions of a
contract — the one currently in production and the one someone wants to
ship — and it tells you exactly which compatibility direction(s) the
change breaks, and blocks the change in CI unless it comes with an
explicit major version bump. It also enforces the same contract at
ingestion time, validating real JSON events against it before they land
in a shared data store.

```
contracts/orders/v1.yaml  ──┐
                             ├─▶ contractgate check ─▶ backward/forward issues ─▶ pass/BLOCKED
contracts/orders/v2.yaml  ──┘         (exit 0 / exit 1 — wire into any CI)

producer A ──┐
producer B ──┼─▶ validate against current contract ─▶ shared store (SQLite)
producer C ──┘                                    └─▶ dead-letter table (rejected events)
```

## Why this exists

A "data contract" is a written agreement about the shape of the data one
team publishes and other teams consume — which fields exist, which are
required, what type each one is, and (for strings) what values are
allowed. The moment more than one team depends on a shared contract, you
have a new failure mode: team A changes the shape of the data (adds a
field, removes one, tightens a rule) and team B's pipeline breaks days
later, in production, with no warning. Nobody did anything obviously
wrong — the change just shipped silently.

The fix people reach for is "have a schema file." That's necessary but
not sufficient: a schema file sitting in a repo doesn't stop anyone from
editing it. What actually stops a breaking change is a mechanical check,
run in CI, that answers one precise question before the change merges:
*does this specific change break anything, and if so, has the author
admitted that by bumping the version?*

Answering that question correctly turns out to be harder than it looks,
because "does this break something" is actually two different questions,
not one (see the next section). contractgate exists to get that reasoning
right every time, mechanically, instead of relying on a human reviewer to
re-derive it by eye on every pull request.

## The core idea, with examples

It's tempting to think of "compatible" as one property a schema change
either has or doesn't. It isn't. There are two separate questions, and a
change can pass one while failing the other.

**Backward compatible** asks: if a consumer upgrades to the *new* schema
today, can it still correctly read data that was already written under
the *old* schema yesterday? This is the question that matters when
consumers upgrade before all the old data has been reprocessed, or when
someone replays historical data through newly-upgraded reader code.

> Example: a contract requires a `status` field. A new version adds a
> second required field, `warehouse_id`. Every row written before the
> change is missing `warehouse_id` — a consumer running the new schema
> would choke on that old data. This breaks backward compatibility.

**Forward compatible** asks the mirror-image question: if a producer
starts writing data under the *new* schema today, can a consumer that
hasn't upgraded yet — still running the *old* schema — correctly read
it? This is the question that matters during an ordinary rolling
deploy, where producers typically ship a new contract before every
consumer has caught up.

> Example: same contract, same new required field `warehouse_id`. A
> consumer still running the old schema has never heard of
> `warehouse_id` and simply ignores fields it doesn't recognize. It reads
> the new data just fine. This does NOT break forward compatibility.

So the exact same change — adding a new required field — breaks backward
compatibility but not forward compatibility. That asymmetry is the whole
point: these are genuinely different questions, and several of the most
common contract changes answer them in opposite ways. Here's the full
picture contractgate computes for every field in a contract:

| Change | Breaks backward? | Breaks forward? |
|---|---|---|
| Add an optional field | No | No |
| Add a **required** field | **Yes** — old data was written without it | No — old readers ignore fields they don't recognize |
| Remove an optional field | No | No |
| Remove a **required** field | No — new readers simply stop looking for it | **Yes** — old readers still expect it on every record |
| Optional field becomes required | **Yes** — old data may be missing it | No — old readers only ever expected "maybe present" |
| Required field becomes optional | No — old data always had it anyway | **Yes** — old readers still expect it every time |
| Enum: remove an allowed value (narrowing) | **Yes** — old data may have used the removed value | No — the new, narrower set of values is still valid under the old rules |
| Enum: add an allowed value (widening) | No — old data never used the new value in the first place | **Yes** — new data may use a value old readers have never seen and will reject |
| A field's type changes | **Yes** | **Yes** |

Look closely at the pairs: "add required field" and "remove required
field" are exact mirror images — one breaks backward and not forward,
the other breaks forward and not backward. The same is true of "optional
→ required" vs. "required → optional," and of "narrow an enum" vs.
"widen an enum." That symmetry is easy to get backwards if you reason
about it casually, which is exactly why it's worth writing down as a
table and testing both directions independently rather than trusting
intuition. (It's also why the project's 35+ tests essentially all passed
on the first run: the table above was worked out carefully on paper
*before* any code was written, so there was nothing left to debug.)

## How it works

Running `contractgate check -old v1.yaml -new v2.yaml` walks through these
steps:

1. **Load both contract files.** Each contract is a small YAML document:
   a name, a semver version (`MAJOR.MINOR.PATCH`), a declared
   compatibility mode (`backward`, `forward`, `full`, or `none` —
   defaults to `full` if left unset, the safest choice), and a flat list
   of fields, each with a name, a type (`string`, `integer`, `number`,
   `boolean`), whether it's required, and — for string fields only — an
   optional list of allowed values (an enum).
2. **Classify every field-level change.** For every field that exists in
   either version, decide independently whether the change to it breaks
   backward compatibility, forward compatibility, both, or neither,
   using exactly the reasoning in the table above. This step doesn't yet
   know or care what the contract's declared mode is — it just produces
   the full, honest list of what changed and which direction(s) each
   change breaks.
3. **Apply the contract's declared compatibility mode.** A contract
   declares what it promises to protect: `backward` only, `forward`
   only, `full` (both), or `none` (no enforcement — meant for contracts
   with no consumers yet). This step filters the full list from step 2
   down to the issues that actually matter under that promise. A
   `backward`-only contract, for example, doesn't care about a change
   that only breaks forward compatibility.
4. **Check the version bump.** A relevant compatibility issue isn't
   automatically forbidden — it has to be paired with an honest signal.
   The rule: if there are any relevant issues, the new contract's
   version must have a strictly greater MAJOR number than the old one
   (a minor or patch bump doesn't count, and a major bump paired with a
   *lower* minor/patch, e.g. `1.5.0 -> 2.0.0`, still counts). This
   mirrors how real schema registries and semantic versioning work in
   general: a minor/patch release is a promise that nothing breaks; a
   major release is the explicit, visible admission that consumers need
   to pay attention before upgrading.
5. **Catch mode-weakening as its own violation.** There's a sneaky way to
   dodge the whole gate: instead of making a breaking change honestly,
   just declare a less strict compatibility mode (say, quietly switching
   from `full` to `none`) so nothing gets checked anymore. contractgate
   treats *that* — weakening the declared mode without a major bump — as
   a blockable violation in its own right. Otherwise "I'll just turn off
   the alarm" would be a quieter way to ship a breaking change than
   actually making one.
6. **Produce a verdict.** If no relevant issue survived step 3, or every
   relevant issue is paired with a major bump, the check exits `0` and
   prints `OK`. Otherwise it exits `1`, prints `BLOCKED`, and lists each
   reason in plain language (which field, which issue, why) so the
   person reading the CI log knows exactly what to fix or which version
   to bump — no guessing from a generic failure message.

## Architecture at a glance

- **`internal/contract`** — defines the contract format (name, semver
  version, compatibility mode, flat field list) and loads/validates a
  YAML contract file: every field needs a name and a recognized type, no
  duplicate field names, enums are only allowed on string fields, and an
  unset compatibility mode defaults to `full`.
- **`internal/semver`** — just enough semantic-version parsing and
  comparison to answer one question: is this a major bump? (Full semver
  pre-release/build-metadata syntax is out of scope; contract versions
  here are always plain `MAJOR.MINOR.PATCH`.)
- **`internal/compat`** — the heart of the project. `Classify` compares
  an old and new contract field-by-field and returns every backward and
  forward issue found (step 2 above). `Evaluate` takes that output,
  applies the declared compatibility mode and the version-bump rule, and
  returns a final pass/fail `Decision` with human-readable reasons
  (steps 3–6 above).
- **`internal/validate`** — the runtime half: checks one decoded JSON
  event against a contract — required fields present, types matching
  (including distinguishing a true integer from a number with a
  fractional part: JSON itself doesn't separate `3` from `3.0` from `3.5`
  at the wire level, so a field declared `integer` explicitly rejects a
  value like `3.5`), and enum membership. Fields not declared in the
  contract are allowed and ignored, since a contract describes a
  required minimum shape, not a closed one.
- **`internal/store`** — a small SQLite-backed ingestion gateway that
  multiple producers write through. Every incoming event is validated
  against the contract first; a conforming event lands in the shared
  `events` table (tagged with exactly which contract version it was
  validated against, so later audits can tell which rules were in force
  for any given row); a non-conforming event is written to a
  `dead_letters` table instead of being silently dropped or, worse,
  silently accepted into the shared table.
- **`cmd/contractgate`** — the CLI, with three subcommands: `check` (the
  CI gate described above), `validate` (run a JSON data file through
  `internal/validate` and report violations), and `demo` (a scripted
  walkthrough that runs a passing check, a blocked check, a couple of
  ingestion attempts, and a rolling-upgrade scenario against the SQLite
  store, all in one command).
- **`contracts/orders/`** — three example versions of one contract used
  by the demo, the tests, and the CI workflow: `v1.yaml` (the baseline),
  `v2-compatible.yaml` (adds an optional field — safe), and
  `v2-breaking.yaml` (removes a required field without a major bump —
  unsafe).

## Using this as a real CI gate

This repo's own `.github/workflows/ci.yml` doesn't just run unit tests —
it runs `contractgate` against itself as a second job
(`contract-gate-self-check`), proving the gate works against a real
binary in a real CI environment rather than only inside a Go test
function:

1. It builds the `contractgate` binary.
2. It runs `check` from `v1.yaml` to `v2-compatible.yaml` and asserts the
   command exits `0` (the change is genuinely safe and must be allowed).
3. It runs `check` from `v1.yaml` to `v2-breaking.yaml` and asserts the
   command exits non-zero (the change genuinely removes a required field
   without a major bump and must be blocked) — if the gate ever let this
   one through, the workflow step itself fails loudly with
   `::error::expected the breaking contract change to be blocked, but
   the gate passed it`.

That second assertion is the important one: it's not just checking that
the tool runs, it's checking that the tool is still correctly blocking a
real breaking change, every time CI runs, forever.

The intended usage in a real producer team's repository is the same
shape, pointed at git history instead of two static example files: run

```bash
contractgate check -old <(git show origin/main:contracts/orders.yaml) -new contracts/orders.yaml
```

as a required PR check whenever the contract file changes. That way a
breaking schema change can never merge without its author consciously
bumping the major version — the check fails the PR instead of the
breakage being discovered later from a downstream incident.

## Try it yourself

```bash
make build
make test        # 35+ tests across semver/contract/compat/validate/store
make demo        # CI-gate decisions + a multi-producer ingestion scenario, end to end
```

```bash
# Adds an optional field — nothing breaks either direction — exits 0
./bin/contractgate check -old contracts/orders/v1.yaml -new contracts/orders/v2-compatible.yaml

# Removes a required field without a major version bump — exits 1, BLOCKED
./bin/contractgate check -old contracts/orders/v1.yaml -new contracts/orders/v2-breaking.yaml

# Validate a JSON data file (one object, or an array of objects) against a contract
./bin/contractgate validate -contract contracts/orders/v1.yaml -data some_events.json
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full design
rationale, including why the contract format is a deliberately flat,
custom one rather than JSON Schema or Avro, and the complete testing
strategy per package.

## What this deliberately doesn't do (yet)

- **No nested objects or arrays as contract fields.** Every field here is
  a scalar (string/integer/number/boolean). Real formats like Avro and
  Protobuf support nested records; the compatibility reasoning for
  nested structures is a generalization of the same rules above, not a
  different problem, but it was left out to keep the field-diffing logic
  small enough to verify by hand.
- **No rename or alias detection.** Renaming a field looks identical to
  "remove field A, add field B" under this model, and is correctly (if
  conservatively) flagged as breaking in whichever direction each half
  implies — the same behavior real schema registries without alias
  support show.
- **Not packaged as a reusable GitHub Action.** `.github/workflows/ci.yml`
  demonstrates the intended usage pattern against this repo's own
  example contracts; publishing it as a composite action other
  repositories can pull in directly is a straightforward follow-up, not
  a new engineering problem.

## Project layout

```
internal/contract/       contract format + loading/validation
internal/semver/          minimal semver parsing/comparison
internal/compat/          compatibility classification + CI-gate decision engine
internal/validate/        runtime event validation against a contract
internal/store/           SQLite-backed multi-producer ingestion gateway
cmd/contractgate/         CLI: check, validate, demo
contracts/orders/         example contract versions (compatible + breaking)
.github/workflows/ci.yml  real CI: tests + a self-check that the gate blocks a real breaking change
docs/ARCHITECTURE.md      design rationale, including the full compatibility table
```

## License

MIT — see [LICENSE](LICENSE).
