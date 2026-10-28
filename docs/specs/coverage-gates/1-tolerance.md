# Step 1 — A measured tolerance under each threshold

**Files:** `sidecar/scripts/coverage.go`, `scripts/coverage-js.go` (new), the `coverage-*` files
beside each, `Makefile`, `.github/workflows/ci.yml`, a new `.github/workflows/coverage-spread.yml`.
**Prerequisites:** none.
**Status:** In progress: PR 1 (the tooling and the workflow) landed as #495; the twenty-run
measurement is done (run 40703375521). PR 2 (the tolerance) is next.
**Pull requests:** two, each into `wip/coverage-gates`. Ref #37; the landing PR closes it.

## Goal

Each gate fails only below `threshold − tolerance`, with the tolerance measured on CI and kept
in a file beside the threshold. The threshold keeps ratcheting to what a change measured.

Read `sidecar/scripts/coverage.go`, the `cover-go` and `cover-js` recipes in the `Makefile`, the
`go-coverage`, `go-coverage-gate` and `typescript-coverage` jobs in `ci.yml`, and #37 first.

## The files

| Gate | Threshold | Tolerance (new) | Statements | One statement |
| --- | --- | --- | --- | --- |
| Go, every file | `sidecar/scripts/coverage-threshold` | `sidecar/scripts/coverage-tolerance` | 22,576 | 0.004 pt |
| Go, files Linux does not compile | `sidecar/scripts/coverage-threshold-platform` | `sidecar/scripts/coverage-tolerance-platform` | 533 | 0.19 pt |
| TypeScript | `scripts/coverage-threshold` | `scripts/coverage-tolerance` | lines, see `cover-js` | |

Each holds one number, in percentage points, and nothing else. The counts are from run
40703375521 on `wip/coverage-gates`; the platform set is small enough that one statement is a
visible step, so it gets its own tolerance.

## Why the spread must be measured uncached

`go test` caches a package's result, coverage profile included, and the `go-coverage` job
restores that cache and dates files with `restore-mtime.go` so it hits. A repeated run replays
the cached packages and measures nothing. The spread cells therefore run with
`GOFLAGS=-count=1`, which the cells set in `env` beside `CGO_ENABLED`. The TypeScript suite
has no result cache.

This means CI's own gate varies less than the spread does: a PR run replays every package the
PR did not touch. The tolerance measured uncached errs on the safe side. The ADR says so.

## Measuring the spread

`.github/workflows/coverage-spread.yml`, started by `workflow_dispatch`, with a `runs` input that
is a JSON array as `stress.yml`'s `cpu` is. The default is `[1,2,3,4,5]`; the measurement passes
twenty. A run is sixty Go cells of up to an hour, twenty on the macOS runner, so the workflow
sets `concurrency` to one run at a time and the ADR records how long twenty took (20m46s).

- A Go matrix over `runs × {linux, darwin, windows}`, each cell the `go-coverage` job's steps
  with `GOFLAGS=-count=1`, uploading `coverage-<os>-<run>.out`. As `stress.yml` does, a cell
  restores CI's Go cache by key and never saves one: the `go-coverage` job's *Save Go cache* step
  is not copied, or sixty cells would push CI's own caches out.
- A TypeScript matrix over `runs`, each cell `go run scripts/coverage-js.go -nogate -summary
  <path>`, uploading the summary.
- A report job that runs `coverage.go -spread` over the Go profiles and `coverage-js.go -spread`
  over the summaries (both below), then checks each run against `threshold − tolerance` with the
  tolerance files as they are on the branch, and fails if any run is below. With no tolerance
  file yet, it reports and does not fail.

`workflow_dispatch` runs only a workflow that exists on the default branch, and this one
reaches `main` only with the landing PR. Until then a `push` to `wip/coverage-gates` starts it,
which is how run 40703375521 ran. Step 2 removes that trigger, so the next spread run waits for
the landing PR.

## `-spread`

`coverage.go -spread -dir <d>` finds every `<run>` for which all three of
`coverage-{linux,darwin,windows}-<run>.out` are in the directory, since the `runs` input is any
JSON array, unions each run's three as `gate` does, and prints:

- Per run: both percentages, as `gate` prints them, plus the exact fraction
  `hit/total` so no rounding enters the arithmetic.
- Over all runs: min, median and max of each percentage.
- Every block (`file:start,end`) that was covered in some runs and not others, with the number of
  runs that covered it. Blocks, not files: step 3 fixes lines.

`coverage-js.go -spread <summary>...` prints per run `total.lines.covered / total.lines.total`
and the percentage computed from them, then min, median, max. `total.lines.pct` in the summary
is already rounded to two decimals and is not used.

**The floor check lives in `-spread` itself.** Once the tolerance files exist (PR 2), each
`-spread` reads them and exits 1 naming every run below `threshold − tolerance`; until then it
only reports. The report job's result is the two `-spread` exits.

## Choosing the tolerance

The ratchet puts the threshold at whatever one run measured, which can be the top of the spread.
So the floor must hold when the threshold is the max and the run is the min:

```
tolerance = (max − min) + one statement (or one line), rounded up to two decimals
```

Today's thresholds were set by CI runs that replayed cached packages, so one could sit above
every uncached run. None does: each is at or below its uncached min.

Run 40703375521, twenty runs on `wip/coverage-gates`, measured:

| Gate | Threshold | Min | Max | Tolerance |
| --- | --- | --- | --- | --- |
| Go, every file | 97.27 | 97.271% (21960/22576) | 97.329% (21973/22576) | 0.07 |
| Go, files Linux does not compile | 98.87 | 98.874% (527/533) | 98.874% (527/533) | 0.19 |
| TypeScript | 97.9 | 98.703% (6014/6093) | 98.703% (6014/6093) | 0.02 |

43 Go blocks move between runs, but together they move the total by 13 statements, far under the
half point that would make the variance a test problem. So no mover is fixed in this step, and
nothing is re-measured. No threshold sits above its uncached max, so none is lowered.
TypeScript's sits 0.8 points under what it measures; PR 2 raises it to 98.70, as the ratchet
does. The ADR records every run's fractions.

## The gates

`coverage.go`: `threshold(name)` gains a sibling `tolerance(name)` reading `coverage-tolerance*`.
`check` and `gate` fail when `pct + 1e-9 < want − tol`, and print the tolerance beside the
threshold:

```
coverage: 97.25% of 22576 statements (threshold 97.27%, tolerance 0.10%)
```

A missing or unreadable tolerance file fails the gate naming the file.

`cover-js` moves from a vitest flag to `scripts/coverage-js.go`, a stdlib root script like the
others (`//go:build ignore`, run with `go run`). It runs `pnpm vitest run --coverage` with the
`text` and `json-summary` reporters, computes the percentage from `total.lines.covered` and
`total.lines.total` in `coverage/coverage-summary.json`, and gates on `scripts/coverage-threshold`
minus `scripts/coverage-tolerance` with the same message shape. The vitest
`--coverage.thresholds.lines` flag goes; vitest only reports. Until PR 2 adds the tolerance,
the gate floors the fraction to two decimals before comparing, as istanbul's `percent()` does
(`Math.floor` of the thousandths), so PR 1 gates exactly as today; PR 2 compares the exact fraction against `threshold − tolerance`. Flags:
`-nogate` measures without gating, `-summary <path>` copies the summary out, `-spread` is above. Its failure message says
*add tests*; it does not suggest lowering the threshold, which step 3B puts behind a label.

The ratchet does not change: a change that lifts coverage raises the threshold to the number it
measured. The tolerance absorbs the next run landing lower.

## Tasks

Each task is its own PR into `wip/coverage-gates`, in this order.

1. **PR 1: the tooling and the workflow.** `coverage-js.go` with `-nogate`, `-summary` and
   `-spread`, and `cover-js` calling it; `-spread` in `coverage.go`; `coverage-spread.yml`. No
   tolerance files yet: `coverage.go` and `coverage-js.go` do not read them until PR 2, and the
   report job only reports. Add `scripts/coverage-js.go` and `scripts/coverage-tolerance` to
   `ci.yml`'s `typescript` path filter beside `scripts/coverage-threshold`. `make lint-yaml`
   already lints every workflow, the new one included. PR 1 targets `wip/coverage-gates`, so
   `coverage-spread.yml` also runs twenty runs on every push to that branch; a dispatch alone
   cannot start it off `main`. Done: #495, measured by run 40703375521. Step 2 removes the
   `push` trigger and its twenty-run fallback (back to `[1,2,3,4,5]`). If PR 2 merges first,
   its push starts a twenty-run spread against the new floors, a second acceptance check.
2. **PR 2: the tolerance.** `tolerance()` in `coverage.go`, both gates, the message; the gate in
   `coverage-js.go`; the three tolerance files with the numbers above; the TypeScript threshold
   raised to 98.70, the run cited in the commit body; the report job now fails a run below the
   floor. The ADR
   `docs/adr/<date>-coverage-thresholds-ratchet-and-gate-with-a-tolerance.md`: the runs'
   fractions, each tolerance and the formula, why the spread is measured uncached and why that
   errs safe, why 100% is not the target (#37's list), the ratchet rule. Status Accepted. Docs:
   the `cover-go`/`cover-js` comments in the `Makefile`, the header of `coverage.go`, the
   *Coverage is gated* paragraphs in `sidecar/CLAUDE.md` and `src/CLAUDE.md`, and the
   `make test-changed` line in the root `CLAUDE.md`; each states the tolerance and links the ADR.
   This PR says `Ref #37`.

## Acceptance

- `make cover-go` and `make cover-js` print the threshold and the tolerance and fail only below
  their difference. A tolerance file that is missing or not a number fails the gate with the
  file's name.
- Every run of run 40703375521 is at or above `threshold − tolerance` for all three gates, with
  PR 2's thresholds and tolerances. `coverage.go -spread` and `coverage-js.go -spread` over
  that run's downloaded artifacts exit 0 on PR 2's branch.
- No threshold goes down.

## When it lands

PR 2 deletes this file and its row in the README, once the docs above state the tolerance.
