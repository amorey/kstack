---
title: Sessions
scope: sidecar
status: Planned
---

# Sessions

**Needs:** step 1B, whose runtime field this step folds in. **Unblocks:** steps 3B, 4C, 4D and
6D, each of which adds a field to the session.

Go paths below are under `sidecar/internal/` unless they say otherwise.

## In short

The note's third principle: one session is one sandbox instance, one proxy token and one
approval policy, and the chat agent, the monitoring agent and a subagent are sessions with
different tokens. Today the pieces of a session are scattered: the Bash tool reads the chat's
cluster and workspace off `tools.Runtime`, a subagent's runtime is assembled by hand in
`chatsvc/subagent.go`, and there is no place to hang a policy on.

After this step, a **session** is one value, `session.Session`, that every tool gets on its
runtime and that every proxy a run serves reads through the run's token:

- its **kind**: `chat`, `subagent` or `monitor`;
- the **chat** and **cluster** it belongs to, and its **workspace**;
- whether the user switched it to run **outside** the sandbox (step 1B);
- and, from later steps, its approval mode (step 3B), its hosts (step 4C), its folders (step 4D), and
  whether it may read Secret data (step 5A).

A chat's turn makes its session. A subagent's session is its parent's, narrowed: nothing a
subagent holds is more than its parent holds, by construction. A `monitor` session is named
here and built in step 6D.

**This step changes no behavior.** It moves fields into one place and adds the type the next
steps fill.

## What is not in this step

- **No policy.** The mode, the rules and `Decide` are step 3B's.
- **No token per session.** A run's token stays one per run, as the note's *Where this meets the
  code* decides; this step maps it to the session.
- **No monitor.** Step 6D builds it.
- Nothing changes on Windows.

## Design

### 1. The `Session` type

A new leaf package, `session`, importing `apimeta` alone:

```go
// Kind is what kind of agent a session runs.
type Kind string

const (
	Chat     Kind = "chat"
	Subagent Kind = "subagent"
	Monitor  Kind = "monitor"
)

// Session is one agent run's identity and policy: what its tools and the
// proxies its runs serve read to decide what it may do. chatsvc builds a chat's
// at the start of each turn; a subagent's is Narrow of its parent's.
type Session struct {
	Kind      Kind
	ChatID    apimeta.ChatID    // empty for a monitor
	ClusterID apimeta.ClusterID // the cluster its cluster tools reach
	Workspace string            // the folder its commands start in and may write
	Outside   bool              // the user switched the chat to run outside the sandbox
}

// Narrow is a subagent's session under parent: the same chat, cluster and
// workspace, and never more than parent holds. Later steps keep this true for
// every field they add.
func Narrow(parent Session) Session
```

A subagent shares its parent's workspace, since its `Write` and `Edit` land there and the
transcript draws them open under the chat; its privilege is its parent's, so a separate folder
would separate nothing. The note's *Where this meets the code* records this.

### 2. The runtime carries it

`tools.Runtime` gains `Session session.Session`, and loses `OutsideSandbox` (step 1B), which
moves to `Session.Outside`. `ClusterID` and `ChatID` stay on the runtime as they are, since
every tool reads them; `Session` repeats them so a proxy handed the session alone has them.

- `chatsvc/turn.go` builds `Session{Kind: Chat, ChatID, ClusterID, Workspace, Outside}` where it
  reads the chat's cluster (`chatCluster`), once per turn.
- `chatsvc/subagent.go` sets the subagent's runtime `Session` to `session.Narrow(parent)`.
- A test's runtime sets the fields its tool reads, as today.

### 3. Bash reads the session

`tools/bash`:

- `sandboxerFor(rt)` reads `rt.Session.Outside` (step 1B read `rt.OutsideSandbox`).
- `sandboxedRunFor` builds the run from the session: the workspace from `Session.Workspace`, the
  cluster from `Session.ClusterID`.
- The run's `kubeproxy.Grant` is made with the session: `NewGrant` gains a `session.Session`
  argument, kept on the grant, which step 3B reads to decide each write. The token stays the
  grant's, one per run: the run's kubeconfig carries it, and the handler that checks it finds
  the session on the grant. That is the map from token to session the note asks for.

### 4. Where the monitor will go

`Monitor` is a `Kind` and nothing else in this step. Step 6D builds the session, with no chat,
its own workspace, and the policy the note gives it. Naming the kind now lets steps 3B, 4B, 4C, 4D and 5A say
what a monitor session does in each of their tables, so nothing is retrofitted.

## Tasks

| # | Task | Files | Needs | Status |
| --- | --- | --- | --- | --- |
| 1 | The `session` package | `session/session.go`, its test | — | Planned |
| 2 | `Runtime.Session`; chatsvc builds and narrows it | `tools/tool.go`, `chatsvc/turn.go`, `chatsvc/subagent.go`, their tests | 1 | Planned |
| 3 | Bash reads the session; the grant keeps it | `tools/bash/bash.go`, `tools/bash/proxy.go`, `kubeproxy/kubeproxy.go`, their tests | 2 | Planned |
| 4 | Docs, per *When it lands* | see there | 1–3 | Planned |

**Order:** 1, then 2, then 3, then 4.

## Tests

**`session`**

- `TestNarrowKeepsTheParentsIdentity`: a narrowed session has `Kind` subagent and every other
  field the parent's. This test grows a case per field later steps add.

**`chatsvc`**

- `TestATurnBuildsItsSession`: a turn's runtime carries `Kind` chat, the chat's id and cluster,
  its workspace and its switch.
- `TestASubagentsSessionIsItsParents`: the subagent's runtime carries `Narrow` of the parent's.

**`bash`**

- `TestTheRunIsBuiltFromTheSession`: over a fake sandbox, the workspace and the cluster the run
  uses are the session's.
- `TestTheGrantKeepsTheSession`: the grant a run serves answers the session it was made with.

Every existing test that set `Runtime.OutsideSandbox` sets `Session.Outside` instead.

## Security

No boundary moves. The subagent's narrowing is the one property this step states and pins: a
subagent's session is its parent's, so nothing a later step hangs on a session can give a
subagent more than its parent. The note's session-token invariant, that a request with an
unknown or dead token is rejected, holds today (`TestAWrongOrDeadTokenIsUnauthorized`) and is
unchanged.

No security record. `security-model.md`'s rows on the subagent gain the narrowing test.

## When it lands

- **`sidecar/CLAUDE.md`**: the `session` package, `Runtime.Session`, where a turn and a subagent
  build it, and the grant keeping it.
- **`security-model.md`**: the narrowing test on the subagent's rows.
- **The sequence's README**: this row's status.

## Verification

Run the [verification commands](../README.md#verification-commands). By hand, `pnpm tauri dev`:
a sandboxed command and a subagent's sandboxed command behave as before, and a switched chat's
commands still ask.
