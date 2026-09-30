# Specs

A spec describes **what we are about to build**, in enough detail to implement it and no more.
It is a plan with a shelf life.

The other two doc kinds keep their jobs: `CLAUDE.md` says what is true now, `docs/adr/` says why a
design was chosen. A spec says what comes next.

## Filenames

```
docs/specs/<n>-short-slug.md   a spec with a place in a sequence
docs/specs/short-slug.md       one that stands alone
docs/specs/<topic>/<n>-slug.md a sequence of its own, with a README.md naming its steps
```

No dates — a spec is edited as the work moves, not appended to. **The number is the build order**,
so the directory listing is the plan; a spec with no place in a sequence goes without one.
Renumber when the planned build order changes, and fix the links in the same edit. Retiring a
completed step does not renumber the remaining steps; their numbers preserve the original order.

## Lifecycle

Update the spec while the work is in progress. When it lands, fold what is now true into the
relevant `CLAUDE.md`, write an ADR if a decision needs its reasons recorded, and delete the spec.
A spec left behind after the code ships is a second source of truth.
A security spec also has a line in [`TODO.md`](../TODO.md#security) and a row in
[`security-model.md`](../security-model.md); its own *When it lands* section says where the row moves.

## Working a numbered spec

The numbered specs share these rules, so each states only what is its own.

Implement a spec after its stated prerequisites have landed. The numbered spec is the unit of
integration and retirement; its tasks are smaller assignments that run in the listed order.
Assign one task at a time with the spec, the applicable `CLAUDE.md`, and the current code.
All tasks start Planned; record their status and implementation commit/PR in the spec as the work
moves.
Each handoff reports changed files, acceptance cases, verification results and remaining failures.

Keep dependent cutover work on the spec's branch until its integration gate passes. Supporting
work should compile and pass focused checks; do not merge an incomplete schema/service cutover.
Do not introduce a generic repository framework, a second DB owner, or dual writes.
Paths in task lists are relative to `sidecar/internal/` unless explicitly rooted elsewhere.

When complete, update current-state documentation and ADRs as the spec's *When it lands* lists,
then delete **that spec only** and remove its row from the index in the same change. Keep later spec numbers
stable: their prerequisites are delivered code and current-state docs, never a retired spec file.

### What every spec inherits

Delivered by the retired specs 1–4 and documented in `sidecar/CLAUDE.md`.

The app owns and injects one `*appdb.DB`; a service's statements are a `sqlstmt.Set` over a table
of `sqlstmt.Statement`s declared `OnWriter`/`OnReader`/`OnBoth`, prepared on `db.Write, db.Read`. Writer transactions use BEGIN IMMEDIATE. Subscribe
before the first read, notify after commit, and use the DB's `Notify`/`Subscribe`. Timestamps are Unix
milliseconds; NULL differs from zero. `appdb.NewID()` generates canonical UUIDv7 IDs, monotonic within one process only, so ids are identity and never a contractual order;
`appdb.ValidateUUID` accepts canonical lowercase RFC-variant UUIDv4/v7 request keys, rejecting nil IDs.
Clients never supply row IDs. An id is identity and never authorizes: knowing one grants nothing,
and a share link is its own row with a random token. A synced row keeps the id it was minted with,
so the cloud stores client-minted ids unchanged and scopes their uniqueness per account. Shared watch folds use `internal/deltafold`.

Clusters are rows in `app.db` (`clusters`, owned by `clustersvc`), addressed by the `ClusterID`
scalar; beehive holds one runtime object per row, named by its id, that the mirror keeps. A
cluster's deletion is a mark: the mirror tears the runtime down, the chat sweeper deletes the
conversations, and the row goes last. Every send checks its cluster's mark inside its transaction
(`ErrClusterGone` → `KSTACK_RECORD_NOT_FOUND`). `conversations.cluster_id` references `clusters(id)`.

Conversations, messages, agent runs, model calls, tool calls and approvals are the seven
application tables of `appdb/migrations/0001_init.sql`, the only schema authority; `chatsvc`
owns all but `clusters`. A send files the user message (carrying the client's UUID request key),
a queued chat run and the empty assistant message in one transaction; the turn claims its run
and settles it with the answer; a message's public status is its run's. A model call's row is
written before the provider is contacted and a tool call's running row before the tool executes,
each a full-row upsert by id; settlement writes every call row of the turn whole, and the next
start closes what a crash left open.

Edit `appdb/migrations/0001_init.sql` under the existing
[pre-release schema policy](../adr/2026-08-29-schema-edit-not-migration.md). The native-tools
cutover (*A native tool is a contract* ADR) changed the stored content
format and the `llm_calls` columns in place: a dev `app.db` from before it is reset.
Tests use temporary databases. Document fresh developer data directories at schema cutovers;
do not delete the user's live database as an implementation step.

### Verification commands

Before any build/test/install in the Linux sandbox run `bash scripts/sandbox-dev-setup.sh`, per
root `CLAUDE.md`. All commands below run from the repository root unless explicitly prefixed.

- **Go checks:** `make test-go`, `make lint-go`, `make vet-go`. During development use focused
  `cd sidecar && go test ./internal/<affected-package>`; broaden before handing off the task.
- **Wire checks:** `cd sidecar && go run github.com/99designs/gqlgen generate`, then from root
  `pnpm codegen`, `pnpm build`, `make test-js`, `make lint-js`. Never hand-edit generated files.
- **Full checks at a spec's integration gate:** `make test`, `make lint`, `make vet`, `make cover-go`,
  `make cover-js`. Do not lower coverage gates. Include codegen/build checks when wire changed.

## Index

The numbered specs 1–6 and 8 have landed and are retired. 7, the function bash on Chat
Completions, was retired by 22, which offers bash as a function on every dialect.

Spec 9 onward is the LLM rewrite, which reorganizes the model, tool and run-loop code, one step
each. Landed: 9 to 23 (10, 12 and 13 to 15 were built without a spec), 24, the fake's knobs, 25,
stored blocks and the foreign strip, 26, provider and request details, 27, context windows and
the length check, 29, chat service hardening, 30, the wiring tests, and 31, prompt caching on
every provider, and 32, a chat can switch to any model (both live checks with real keys are in
[`TODO.md`](../TODO.md)). Still to come:

- 28 ports what is left of the tests the rewrite's purge removed.

KubeQuery's own sequence, four steps, has landed and is retired; the
[KubeQuery note](../notes/kubequery.md) describes what it built.

The rest stand alone. Each is deleted, and its row removed, in the change that lands it.

| Spec | Scope | Status |
| --- | --- | --- |
| [Sandbox, credentials and permissions](agent-security/README.md), a sequence of its own | sidecar, host, webview | Planned |
