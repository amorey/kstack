---
title: The settings file
scope: sidecar
status: Planned
---

# The settings file

**Needs:** nothing beyond `main`. **Unblocks:** steps 2D, 3A and 3B, which each add fields, and
through them 4C, 4D, 6A, 6D and 7A.

Go paths below are under `sidecar/internal/` unless they say otherwise.

## In short

Today the sandbox has no settings of its own. [The note](../../notes/sandbox-credentials-and-permissions.md)
gives the user a list of things to decide — the frozen `PATH`, the approval modes and rules, the
hosts, the granted folders, the registered tools, the excluded credentials — and each needs a file.

After this step, **`sandboxconfig`** is that file: `<data>/sandbox.json`, a `Store` that opens
it, reads it, writes it under one lock and publishes each write. `Settings` has no fields yet.
Each later step adds its own, and this step fixes how every field is read back and checked, so
the steps can land in parallel without meeting in the struct. Nothing the user sees changes.

## What is not in this step

- **No field.** The table in §2 says which step adds which.
- **No settings on the wire.** The `sandboxSettings` query is step 7A's; `sandboxPath` and the
  `permission…`, `network…` and `credential…` names are their steps'. This step's one query is
  `sandboxRefused`, what the file lost (§3).
- **No Settings section.** Each step that adds a field draws it.

## Design

### 1. `sandboxconfig.Store`

`sandboxconfig/store.go`. The file is `<data>/sandbox.json`, 0600, written through
`atomicjson`. It lives in the data directory, which no sandboxed command reads
(`bash.Paths.DeniedDirs`), and it is not synced: it describes this machine. The `Store` is
modelled on `cloud/prefs` without its envelope:

```go
// Settings is the sandbox's settings. Each field is added by the step that
// needs it; see the table in the spec.
type Settings struct{}

// Store keeps Settings in one JSON file and publishes each write. Safe for
// concurrent use.
type Store struct{ /* the path, a mutex, the current value, a watch.Hub */ }

// Open reads file. A missing file is empty Settings; one that is not a JSON
// object is an error naming the file, since a sandbox with no settings is one
// the user cannot see.
func Open(file string, opts ...Option) (*Store, error)

// Refused is what Open left out of the file, each value with its reason, for
// the store's life.
func (s *Store) Refused() []Refusal

// Get is a copy of the current Settings.
func (s *Store) Get() Settings

// Update applies fn to a copy under the lock and saves it before returning;
// an error from fn saves nothing, and a result equal to the current value
// writes and publishes nothing. fn must not call the store: it runs under
// the lock.
func (s *Store) Update(fn func(*Settings) error) error

// Subscribe is a current-on-subscribe receiver, as prefs has; close it when done.
func (s *Store) Subscribe() *watch.Receiver[Settings]
```

`Get` and each `Send` copy, so no caller reaches the store's value. The copy is a JSON round
trip (`clone` in `store.go`): later fields add slices and maps, and a copy that needs no line per
field cannot miss one. `Settings` holds only JSON-safe values, so the round trip cannot fail, and
`clone` panics if it does. An empty slice or map comes back nil, since `omitempty` leaves it out. `gochan/watch` hands one sent value to every receiver, so receivers share a
delivery: a receiver treats it as read-only, and `Get` is the way to a copy it can change.
**Equal means the same JSON**: `Update` marshals the result and compares the bytes with the
current value's. `cloud/prefs` compares with `reflect.DeepEqual`, which tells a nil slice from an
empty one and would make an `Update` that changes nothing write. A fresh file is written by the first `Update` that
changes something, not by `Open`. The receiver is `gochan/watch`'s, which holds the latest value:
a slow receiver sees the newest `Settings` and may skip one between.

`atomicjson.Load` answers a parse error without the path, so `Open` wraps it with the file's
name. `Option`'s one constructor is unexported: it is the test seam §3 uses.

### 2. The fields, by step

| Field | Step |
| --- | --- |
| `Path` | 3A |
| `DefaultMode`, `Modes`, `Rules` | 3B |
| `Credentials` | 2D |
| `Hosts` | 4C |
| `Folders` | 4D |
| `Tools` | 6A |
| `Monitor` | 6D |
| `Onboarded` | 7A |

Every field is left out of the file when empty, so the file holds only what the user set: a
slice, a map or a string is `omitempty`; a struct, which `omitempty` never leaves out, is
`omitzero`. A file written before a step landed reads under it, since a missing key decodes to
the zero value.

### 3. Read-back

Every field follows one convention, which its step's tests pin. `sandboxconfig/check.go` holds
`checks`, a slice of `func(*Settings) []Refusal`, one per field, empty in this step; each field's
step adds its line. A check removes from the `Settings` every value it refuses and answers one
`Refusal` per value. A `Refusal` is the field, the value — what a row in that field's Settings
section would name it by (a rule's text, a folder's path), else its JSON — and the reason, in
the user's words. The store runs the slice it was given, `checks` in production and a test's own through an
unexported option, which is how this step's tests exercise the convention before any field has a
check.

**A check reads the value alone**: its shape, never the disk or another service. A rule that
needs either — step 4D's folder that must exist, step 2D's profile that must be one `Discover`
found — is its step's, run where the step says (the mutation, or a run's start), and shown by
that step beside what `Refused()` lists.

- **Decoding is per field.** `Open` reads the file as a `map[string]json.RawMessage` and decodes
  each key into the `Settings` field whose JSON name, the tag less its options, is that key
  exactly. A key whose value does not decode into its field's type — a string where a list
  belongs — is refused whole, as one `Refusal` naming the field with the raw JSON as its value,
  and the other fields load. A struct field is one key, so one bad member refuses the whole
  struct. An unknown key is ignored, and the next write drops it. Only a file that is not a JSON
  object fails `Open`, `null` included. So a hand edit can cost a field, never the app.
- **On `Open`**, a value the check refuses is left out of the loaded `Settings`, logged as one
  line per value, and kept on the store: `Refused()` lists them for the store's life, so a
  Settings section can show the value and its reason. A file refused in part still opens. The
  next `Update` that writes, of any field, writes what `Settings` holds and so drops the refused
  values from the file; `Refused()` still lists them until the sidecar restarts. That loss is
  deliberate: the same check would refuse the value at every launch, and the log line and this
  launch's Settings section are where the user learns of it.
- **On the wire**, `sandboxRefused: [SandboxRefusal!]!` answers `Refused()`, with
  `type SandboxRefusal { field: String!, value: String!, reason: String! }`, `field` being the
  JSON key. It is a query, not a watch, since the list is fixed at `Open`. Each later step's
  Settings section reads it and draws the entries of its own field, so no step's own query
  carries refusals, and steps 2D, 3A and 3B, landing in either order, need nothing of each
  other to show them.
- **On `Update`**, the same check runs over the result, and a refusal is the error: nothing is
  written, and the caller answers `KSTACK_VALIDATION_ERROR` with the reason.

### 4. The app opens it

`app.paths` gains `SandboxFile`, `<data>/sandbox.json`. `app.New` opens the store right after
`app.db`, on every platform, and closes nothing for it: the store holds no handle. A failed
`Open` fails `New` through its `fail` path, so `app.db` is closed.
`graph.Resolver` gains `SandboxCfg *sandboxconfig.Store`, never nil, like every resolver field
(named for what it holds, since the store is not a service);
its one resolver in this step is `sandboxRefused`. A resolver that answers *no sandbox* keys on
the machine's sandbox status (step 1B's `SandboxStatus`), not on the store. On native Windows the
store opens like anywhere else, since step 7A's `Onboarded` flag needs a home there.

## Tasks

| # | Task | Files | Needs | Status |
| --- | --- | --- | --- | --- |
| 1 | `sandboxconfig`: `Settings`, `Store`, per-field decoding, `check`, `Refusal` | `sandboxconfig/store.go`, `sandboxconfig/check.go`, their tests | — | Planned |
| 2 | `SandboxFile`; `app.New` opens the store; `Resolver.SandboxCfg`; `sandboxRefused` | `app/paths.go`, `app/app.go`, `sidecar/graph/resolver.go`, `sidecar/graph/schema.graphqls`, generated code, their tests | 1 | Planned |
| 3 | Docs, per *When it lands* | see there | 1, 2 | Planned |

**Order:** 1, then 2, then 3.

## Tests

**`sandboxconfig`**

- `TestTheStorePersists`: what `Update` writes is what a reopen reads; a missing file opens empty.
- `TestTheFileIsOwnerOnly` (`store_unix_test.go`): the file lands 0600.
- `TestABadFileFailsOpen`: a file that is not a JSON object — `{`, `[]`, `null` — is an error
  naming the file.
- `TestAFieldOfTheWrongTypeIsRefusedAlone`: over a test's `Settings` with two fields, a file
  whose first holds the wrong JSON type opens with the second loaded and one `Refusal` for the
  first; an unknown key is ignored; a key spelled in another case is unknown.
- `TestAnEqualUpdateWritesNothing`: an `Update` that swaps a nil slice for an empty one writes
  and publishes nothing.
- `TestUpdateIsOneWrite`: one `Update` is one save, an error from `fn` saves nothing, an
  `Update` that changes nothing saves nothing, and of two concurrent `Update`s the later sees the
  earlier's result.
- `TestSubscribeSeesTheLatestWrite`: a receiver gets the current value on subscribe, then the
  value after an `Update`, a copy the test can change without changing the store's.
- `TestARefusedValueIsLeftOutAndListed`: over a test's check that refuses the first time it runs,
  `Open` logs the refusal and lists it in `Refused()`; an `Update` over a check that refuses is
  an error that writes nothing; the next `Update` that writes drops the value from the file, and
  `Refused()` still lists it. Each field's
  step adds its own case over its real check (step 3B's `TestABadRuleIsLeftOutWithItsReason`).

**`graph`**

- `TestSandboxRefusedIsWhatOpenLeftOut`: over a store opened with a test's refusing check, the
  query answers each refusal's field, value and reason.

**`app`**

- `TestABadSandboxFileFailsNew`: `New` over a `SandboxFile` that is not a JSON object fails naming the
  file, and closes what it had built. The store opens whatever the sandbox's probe finds, since
  nothing in `app` keys on it.

## Security

No boundary moves. The file lives in the data directory, which a sandboxed command cannot read,
so nothing a sandboxed command runs can read or change what the user decided. A command outside
the sandbox, and every command on native Windows, runs as the user and can, as it can `app.db`;
it asks first, and the store reads the file only at start, so a change made under it is seen at
the next launch or overwritten by the next `Update`. `security-model.md`'s
owner-only files row gains `sandbox.json`, pinned by `TestTheFileIsOwnerOnly`. No security record.

## When it lands

- **`security-model.md`**: `sandbox.json` joins the owner-only files row.
- **`sidecar/CLAUDE.md`**: `sandboxconfig` under `internal/` (the file, the `Store`, the
  read-back convention, per-field decoding, `sandboxRefused`, and that each field's step names
  it); `SandboxFile` in `app.paths`'
  tree comment; `Resolver.SandboxCfg`.
- **The sequence's README**: this row's status.

## Verification

Run the [verification commands](../README.md#verification-commands), including the wire checks.

By hand, `pnpm tauri dev` on any platform: write `{` into `<data>/sandbox.json` and relaunch,
and the sidecar fails to start naming the file; remove it and relaunch clean.
