# Step 2 — `make cover-changed`

**Package:** `tools/coverchanged` (new, in the `tools/` Go module), `scripts/coverage-js.go`,
`Makefile`, `scripts/test-changed.sh`, `.github/workflows/ci.yml`,
`.github/workflows/coverage-spread.yml`.
**Prerequisites:** step 1's PR 1 (#495) on `wip/coverage-gates`: `scripts/coverage-js.go` and
`coverage-spread.yml`. Nothing on `main`.
**Status:** In review.
**Pull request:** into `wip/coverage-gates`. Ref #97.

## Goal

One command lists the changed lines tests do not reach, as `file:lines`, and exits 0 when the
change is fine. CI runs the same command on every PR and blocks it on that list.

Given the same diff and the same profiles, local and CI print the same list. A fresh local run
is a preview, not a promise: it measures one OS where CI unions three, CI builds with
`CGO_ENABLED=0` and its sandbox setup, and a timing-dependent line can be reached in one run and
not the next. The changed-line gate has no tolerance, so a changed timing-dependent line needs a
deterministic test, or step 3A's marker with a reason.

Read `scripts/test-changed.sh` (how the changed set and the base are found),
`sidecar/scripts/coverage.go` (the profile format), `scripts/coverage-js.go`, `vite.config.ts`'s
`coverage` block, and #97 first, all as they are on `wip/coverage-gates`.

## Where it lives

`tools/` is a Go module with a `go.mod`, so the parsers get tests beside them, as every unit
does; a root `scripts/*.go` file cannot. The command is `go run ./coverchanged` from `tools/`,
and the recipe is:

```make
cover-changed:
	@cd tools && go run ./coverchanged $(ARGS)
```

The command works from the repository root (`git rev-parse --show-toplevel`), wherever it is
started.

Files: `main.go` (flags, measure, print), `diff.go` (the changed lines), `goprofile.go`,
`lcov.go`, each with a `_test.go`. Step 3A adds `ignore.go`.

`goprofile.go` parses with `golang.org/x/tools/cover`, which the module already holds through
`staticcheck`, and unions as `sidecar/scripts/coverage.go` does: that file is a build-ignored
`main` package in another module, so `tools/` cannot import it. The union keeps `coverage.go`'s
check that the darwin and windows profiles add statements outside Linux, which catches a swapped
or duplicated profile.

## The changed lines

The base is as `test-changed.sh` finds it: `BASE`, else `upstream/main` when it exists, else
`origin/main`; `-base <ref>` overrides it, which CI uses. While this sequence is built, a local
run passes `BASE=upstream/wip/coverage-gates` (or `origin/…` without an upstream remote).

The changed lines are the `+` lines of the diff from `<merge-base>` over committed, staged and
unstaged changes, plus every line of an untracked file (`git ls-files --others
--exclude-standard`, as `test-changed.sh` lists them). The diff ignores the developer's git
settings, so its output always parses:

```
git -c core.quotePath=false diff -U0 --no-renames --no-color --no-ext-diff --no-relative \
    --diff-algorithm=myers --indent-heuristic --src-prefix=a/ --dst-prefix=b/ <merge-base>
```

The algorithm is pinned because a developer's `diff.algorithm` can change which of two
identical lines counts as added, and CI's git uses the default.

`--no-renames` makes a renamed file's lines changed lines, as a moved line is: that is the PR's
own code. So a pure `git mv` of a file with uncovered lines fails until they get tests or step
3A's markers. That is intended. A deleted line is nothing to check. Only these files count:

- `sidecar/**/*.go`, not `_test.go`.
- `src/**/*.ts` and `*.tsx`, not `*.test.ts(x)`.

## The measurements

The command runs in one of two modes. By default it measures; with `-no-run` it reads only the
inputs it is given and never starts a test.

By default it measures the suites of the languages that have changed files, then intersects.
It measures nothing when no counted file changed, so a `src/`-only change never runs the Go
suite:

- Go: `cd sidecar && go run scripts/coverage.go -out <tmp>`. The whole suite, both tag variants
  (README decision 7).
- TypeScript: `go run scripts/coverage-js.go -nogate -lcov <tmp>`. `coverage-js.go` gains
  `-lcov <path>`, which adds the `lcovonly` reporter and copies `coverage/lcov.info` to the
  path before any gate, as `-summary` does. Both runs go through `coverage-js.go`, so local and
  CI start vitest the same way.

Each measurement's output, the suites' and the scripts' own summaries, goes to the command's
stderr, so stdout carries only the list. On a failed measurement the command exits 1 with that
output already shown.

CI passes `-no-run` and the inputs the coverage jobs uploaded: the `-linux`, `-darwin` and
`-windows` profiles, unioned as `coverage.go` unions them, and `-lcov <file>`. The inputs per
language are optional, since a PR that touched only one language runs only that language's
jobs. With `-no-run`:

- The three Go profiles go together; one or two is a usage error.
- A path given that does not exist is an error naming it. This is how a coverage job that failed
  before uploading shows up.
- An input that is empty or malformed is an error naming it: a Go profile with no `mode:` line
  or no block, a line that does not parse, an lcov with no `SF:` record. Since a file absent
  from the inputs is skipped, an empty input would otherwise pass.
- A changed file of a language with no input given is an error, not a pass, and so is a run
  with no inputs at all and any changed file.

Without `-no-run`, an input flag is a usage error: a measuring run reads only what it measured.

### Go

A Go profile block is `<import path>:line.col,line.col stmts count`. The file is the import path
with `github.com/kstackhq/kstack/` cut off, which leaves `sidecar/…`, the diff's path.

A changed line is **not covered** when at least one block spans it and every block spanning it
has count 0 in every profile given. A line no block spans is not a statement and is skipped.

A changed file with no block in any profile given has no statements to check: a file of types
or constants, a generated file `coverage.go` drops, or a file no measured build compiles
(`tools.go`, `sidecar/scripts/*.go`, `seccomp_linux_arm64.go`). It is skipped.

When the command measures, one case gets a note. `go/build`'s `Context.MatchFile` is asked, for
the untagged and the `debug` build, whether a changed file compiles on this machine
(`runtime.GOOS`, `runtime.GOARCH`) and on each of CI's coverage targets: linux/amd64,
darwin/arm64 and windows/amd64, the `go-coverage` matrix's runners, listed in `main.go` with a
comment pointing at the matrix. The CI targets' contexts set `CgoEnabled = false`, since CI
measures with `CGO_ENABLED=0`. When this machine compiles it in neither build but a CI target
does, it prints `not measured on <GOOS>`. A file no CI target compiles is skipped silently,
since CI does not check it either.

### TypeScript

An lcov `SF:` path is made relative to the repository root, the root cut off when it is
absolute. A `DA:line,count` of 0 is not covered; a line with no `DA` record is skipped. A
file or line recorded twice is covered when any record's count is above 0, as Go's union is. A
changed file absent from the lcov is one `vite.config.ts` excludes, or holds only types, and is
skipped.

## The output

One line per contiguous range, sorted by path then line, to stdout:

```
sidecar/internal/agent/chat/turn.go:88-91    not covered
src/lib/chat-outbox.tsx:212                  not covered
```

Then, when any line was not covered, one line to stderr and exit 1:

```
cover-changed: 5 changed lines are not covered; add tests that check their behaviour
```

Notes go to stderr and never fail the run:

```
cover-changed: sidecar/internal/app/launchshell_windows.go not measured on linux; CI checks it
```

Nothing on stdout and exit 0 when every changed line is covered. No percentages.

## CI

- `typescript-coverage`: `make cover-js ARGS="-lcov $RUNNER_TEMP/lcov.info"`, then upload it as
  `typescript-coverage` with `if: ${{ !cancelled() }}` and `if-no-files-found: ignore`, so a
  failed total gate still hands over its lines.
- A new job `coverage-changed`, *Coverage · Changed lines*, `ubuntu-24.04`, on `pull_request`
  only. It `needs` `go-coverage` and `typescript-coverage` and runs when either ran:
  `!cancelled()` and not both skipped. The `needs` result decides each language's inputs:
  - **skipped:** the path filter saw no change to that language, or its lint or vet job
    failed. Nothing is downloaded and no flag is passed, so a changed file of that language
    fails the run. After a filter gap that is the failure; after a lint or vet failure,
    `ci-result` already fails on it.
  - **success or failure:** the download step for that language runs, with
    `continue-on-error: true` since a job that failed early uploaded nothing, and the flags are
    passed. A missing file then fails the run naming it. A `typescript-coverage` that failed
    only its total gate still uploaded the lcov, so its lines are listed.

  Steps: checkout with `fetch-depth: 0`; download `go-coverage-*` (`merge-multiple`) when
  `needs.go-coverage.result != 'skipped'`; download `typescript-coverage` when
  `needs.typescript-coverage.result != 'skipped'`; then one step builds the flags from the same
  two results and runs `go run ./coverchanged -no-run -base origin/$GITHUB_BASE_REF` with them.
- Add it to `ci-result`'s `needs`.
- `tools/` gets the checks `sidecar/` gets: Go · Lint runs `gofmt -l` over `tools/` too,
  Go · Vet runs `go vet ./...` and `go test ./...` in `tools/`, and `scripts/staticcheck.sh`
  adds one run over `tools/`. `make lint-go` and `make vet-go`
  match. The `go` path filter already has `tools/**`.
- `coverage-spread.yml` loses its `push` trigger on `wip/coverage-gates` and its twenty-run
  fallback, back to `[1,2,3,4,5]`, as step 1 says.

## Tasks

1. **`diff.go`:** the base rule, the hunks, the file filters. Tests on a fixture repository
   made in `t.TempDir()`: a committed, a staged, an unstaged and an untracked change, a
   deletion, and a rename; the same diff with `color.ui=always`, `diff.noprefix=true` and a
   non-ASCII file name set in the fixture's config.
2. **`goprofile.go` and `lcov.go`:** parse, union, span, the path mapping. Tests on inline
   fixtures, including a file with no blocks, an absolute `SF:` path, and empty and malformed
   inputs of each kind.
3. **`main.go`:** flags, both modes, the `MatchFile` note, the output. The work is a function
   taking the arguments, stdout, stderr and the measuring step as parameters, so a test drives
   the whole command with a fake measurement that writes to the writer it is given. Tests: a
   covered change leaves stdout empty and exits 0 while the fake measurement prints; a
   `src/`-only change measures only TypeScript, and no counted change measures nothing; with
   `-no-run`, no inputs, two of three Go profiles, and a missing input path each fail without
   calling the measuring step.
4. **`coverage-js.go`:** `-lcov`, with a test beside the existing ones.
5. **`Makefile`, `test-changed.sh`:** the recipe; `test-go` and `test-changed.sh` (when `tools/`
   changes) run `cd tools && go test ./...`; `lint-go`, `vet-go` and `staticcheck.sh` cover
   `tools/`.
6. **CI:** the lcov upload, the job, `ci-result`, the `tools/` checks, the spread trigger.
7. **Docs:** the `cover-changed` recipe comment; the root `CLAUDE.md` *Testing conventions*
   bullet gains *run `make cover-changed` until it exits 0 before opening a PR*.

## Acceptance

- `cd tools && go test ./...` passes, and CI runs it.
- On a branch that adds an untested function, `make cover-changed` lists its lines and exits 1;
  after a test is added it prints nothing to stdout and exits 0.
- Adding a field to a struct in a statement-free file (`sidecar/internal/llm/model.go`) prints
  nothing and exits 0, locally and in CI.
- Changing `sidecar/internal/app/launchshell_windows.go` on Linux prints the stderr note and
  exits 0. Changing
  `sidecar/scripts/coverage.go` or `sidecar/tools.go` prints nothing and exits 0.
- Given the profiles a CI run uploaded, `make cover-changed ARGS="-no-run -base <base> …"`
  prints exactly the `file:lines` *Coverage · Changed lines* printed, run in a clean working
  tree checked out at the commit CI measured (the PR's merge commit, `refs/pull/<n>/merge`)
  with `<base>` the same base commit.
- PRs into `wip/coverage-gates`:
  - one touching only `sidecar/` runs *Coverage · Changed lines* on the Go profiles alone;
  - one touching only `src/` runs it on the lcov alone;
  - one with an untested line is red on it, listing that line;
  - one with a failing Go test is red on it too, naming the missing profile.

## When it lands

The root `CLAUDE.md` *Testing conventions* and the three coverage paragraphs name the command.
`docs/TODO.md` gains the branch-gate row (README decision 4).
