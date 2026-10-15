---
title: Page cursors and the window
scope: sidecar, frontend
status: Planned
---

# Page cursors and the window

**Needs:** step 1A, and step 1B as the one commit `feat(log-viewer): build the headless core
over a client contract` on `log-viewer-1b-rework`. **Unblocks:** step 2. Takes in step 1C,
whose opaque cursor is built here as the carrier of the paging hint.

Go paths are under `sidecar/internal/`; TypeScript paths are under `src/`.

## In short

Step 1B's core works and is tested, and it is hard to follow and slow to page backward for one
reason: the webview owns the paging position. It decides whether a page keeps or drops its edge
line, mints moments off its own clock, and carries a five-second overlap to cover what the
sidecar knows and it does not. This step moves the position into the sidecar's cursors and
rebuilds the core around a pure window.

Three changes to the sidecar, each a pull request of its own into `wip/log-views`:

1. **A page answers both edges.** `logsFetch` takes one `cursor` and a mode (`Head`, `Tail`,
   `Around`) and answers `before` and `after`, each a cursor and whether there is more that way.
   The core continues from a page's cursor and never from a line's.
2. **A follow sends bookmarks.** Every two seconds the follow sends the cursor it is safe to
   resume from. The core resumes from the last bookmark, and the overlap lives in the sidecar.
3. **A page cursor carries a paging hint.** Opaque base64 JSON, so nothing outside
   `services/logs` can read one by accident and a later sidecar can add a key, and on a
   page's edges one entry per stream saying where that stream's log starts and how fast it
   writes, so the next backward page sizes its first read right.

One change to the webview, one pull request replacing #228 and #229:

4. **The core is a window plus an adapter.** `window.ts` is a plain class holding the lines,
   the edges, the pin and the pause, taking events and answering what the scroller must do.
   `core.tsx` is the React adapter: the virtualizer, the scroll attribution, the landing.
   `moment.ts` and `before-paint.ts` go. The seeded walk runs on the window.

After this step nothing on screen changes. Step 2 writes the client over the new wire.

## What is not in this step

- **No screen.** Step 2 draws the rows and writes `logs-client.ts`.
- **No session.** A sidecar that resolves a view once and keeps per-stream state for the
  viewer's life is the next step up in paging speed. The hint in the cursor is its stateless
  version, and the wire it leaves is the one a session would keep.
- **No change to how the follow orders lines.** Per container, as step 1A has it.
- **No pin from input events.** The adapter keeps 1B's scroll attribution (`useOwnScroll`).
  Reading intent from wheel and keyboard instead is an experiment for after this step.
- **No eviction.** The window grows at both ends as it does now. Keys are stable, so a later
  step can evict at the far edge without changing the contract.

## Design

### 1. The wire

```graphql
"Which page of a view to read. `Head` reads oldest first after `cursor`, from the start with none;
`Tail` reads newest first before `cursor`, from the end with none; `Around` reads `limit` lines
before the moment `cursor` names and `limit` from it on."
enum LogsFetchMode {
  Head
  Tail
  Around
}

"One edge of a page: where the next page that way continues from, and whether there is one."
type LogsPageEdge {
  "Opaque. A page's cursor, never a line's: it carries what the next read needs."
  cursor: String!
  more: Boolean!
}

type LogRecordsPage {
  records: [LogRecord!]!
  before: LogsPageEdge!
  after: LogsPageEdge!
  "For `Around`: the index of the first record at or after the moment, `records.length` with none. Null otherwise."
  anchor: Int
  skipped: [String!]!
}

extend type Query {
  logsFetch(id: ClusterID!, view: LogsViewInput!, mode: LogsFetchMode!, cursor: String, limit: Int!): LogRecordsPage!
}

type LogsFollowFrame {
  type: LogsFollowFrameType!
  record: LogRecord
  "On `Bookmark`: every line before it has been sent. A follow opened `after` it misses none."
  cursor: String
  reason: String!
}

enum LogsFollowFrameType {
  Line
  Bookmark
  Ended
}
```

`since`, `until`, `after` and `before` go. `LogRecord.cursor` stays: it names the line, and the
webview drops a repeated line by it. A line's cursor and a page's cursor are different texts (§3),
and only a page's is ever sent back as a fetch's `cursor`. `logsFollow` keeps `after: String`,
which the webview always sends.

**The edges, by mode.** `start` is the moment the fetch began on the sidecar's clock.

| Mode | `before` | `after` |
| --- | --- | --- |
| `Head`, no cursor | the oldest record's position, `more: false` | see *the live end* |
| `Head`, cursor | the cursor given, `more: true` | the cut when the limit or a stop cut the page, `more: true`; else *the live end* |
| `Tail`, no cursor | the cut, `more: true`; else the oldest record's position, `more: false` | *the live end* |
| `Tail`, cursor | as `Tail` with none | the cursor given, `more: true` |
| `Around` | the `Tail` half's `before` | the `Head` half's `after` |

**The live end** is a page that read every stream to its end: `cursor` is the moment
`start - followOverlap` (§2) and `more: false`. It is a moment and not the newest record's
position because streams finish their reads at different times: a quiet container can write a
line stamped before the newest record after its own read ended. A follow or a page opened after
that moment re-sends a few seconds of lines the webview holds, which it drops by cursor.

**The cut** is `trimToStops`' position as today: the last record kept, or the nearest stop of a
stream that ran short, whichever is nearer. A page with no record and `more: true` is still the
honest answer for a read that spent its budget.

**`Around`** is two reads inside one `Fetch`: a `Tail` read with `until` the moment, exclusive,
and a `Head` read with `since` the moment, inclusive, each at `limit`, each over its own
readings and budget; `records` is the first's then the second's, `anchor` is the first's
length. `readOptsOf` keeps the inclusive bound for this one case; nothing on the wire is
inclusive any more.

### 2. The follow's bookmarks

`follow.run` already runs a timer for the handoff. Beside it, every `followBookmarkEvery`
(2 s, a field on `Service` a test shrinks) it sends
`FollowFrame{Type: FollowBookmark, Cursor: moment(now - followOverlap)}`, never earlier than
the last bookmark sent and never earlier than the `after` it opened with. `followOverlap` is
5 s, a constant in `follow.go`: the five seconds step 1B keeps as `REOPEN_OVERLAP_MS`, moved to
the one process that sees every stream. A follow opened `after` a bookmark skips every
container's lines up to its moment, as it does today, so what it re-sends is at most the
overlap, held already and dropped by cursor.

A bookmark says nothing about ordering: lines still arrive per container.

### 3. The cursor

The shape is Kubernetes' own `continue` token, base64 of `{"v":1,…}`, which a client passes
back and never reads. `cursorOf` writes a line's position as the standard
base64 of one JSON object, keys in this order, no whitespace:

```json
{"v":1,"t":"2026-10-08T10:00:00.123456789Z","ns":"team-a","pod":"api-7f9c","c":"app","prev":false,"n":0}
```

A moment is the object with `t` alone. **A page's edge cursor is a position or a moment with
one more key, `s`**, the hint: one entry per stream of the fetch, in the order `resolve`
answered them.

```json
{"v":1,"t":"…","s":[{"ns":"team-a","pod":"api-7f9c","c":"app","prev":false,"first":"2026-10-07T22:14:03.1Z","spl":0.42}]}
```

`first` is the moment of the stream's first line as `firstTimestamp` read it, empty when it did
not. `spl` is seconds per line: the span the stream's lines in this page cover, divided by one
less than their count; `0` when the page held fewer than two of its lines, in which case the
entry carries the value the fetch's own cursor carried for that stream, if any. The core never
reads a cursor, so a page of 500 lines carries 500 line cursors of about 110 bytes and two edge
cursors of about 100 bytes a stream.

`parseCursor` reads a bare RFC 3339 time as a moment, as it does now, since the `At` anchor
sends one; anything else is base64 of the object, refused with `errBadCursor` when it is not,
when `v` is not 1, when `t` is not a time, when `c` is present and empty, or when `n` is
negative. A key it does not know is ignored. It answers the `position` and the hints,
`[]streamHint`, which `Fetch` hands each `readOpts` by stream key.

**A hint is a hint.** `readBackward` uses `first` in place of the `firstTimestamp` GET, and
`backwardUntil` sizes its first reach as `max(1s, spl × limit × 2)` instead of the span over
2^14, capped at the span. A hint that is wrong must cost GETs, never lines: `first` earlier than
the log's real first line, which rotation can make it, adds windows that read nothing; `spl`
off by any factor changes how many windows it takes. `fetcher_test.go` pins both with a hint
that lies.

**What a before-page costs**, for the quiet log a day old that spec 1A's windows handle worst:

| | per stream, today | per stream, with the hint |
| --- | --- | --- |
| GETs | up to 16 | 1 to 2 |

### 4. The client contract

`src/lib/log-viewer/types.ts` becomes:

```ts
export type Cursor = string;

export type LogLine = { /* unchanged */ };

export type Anchor = { kind: 'Head' | 'Tail' | 'At'; at: string | null };

export type FetchMode = 'Head' | 'Tail' | 'Around';

export type FetchQuery = {
  mode: FetchMode;
  /** A page's edge cursor, or for `Around` the moment the anchor names. Null reads from the start or the end. */
  cursor: Cursor | null;
  limit: number;
};

export type PageEdge = { cursor: Cursor; more: boolean };

export type Page = {
  lines: LogLine[];
  before: PageEdge;
  after: PageEdge;
  /** For `Around`: the index of the first line at or after the moment, `lines.length` with none. */
  anchor: number | null;
  skipped: string[];
};

export type FollowFrame = { kind: 'Line'; line: LogLine } | { kind: 'Bookmark'; cursor: Cursor };

export type Client = {
  fetch: (query: FetchQuery) => Promise<Page>;
  /** Calls back per frame written after `after` until the function it answers is called. */
  follow: (after: Cursor, onFrame: (frame: FollowFrame) => void) => () => void;
};
```

The library mints no cursor and reads none. The one cursor it passes that no page gave it is
the `At` anchor's moment, which is the view's own word.

### 5. The window

`src/lib/log-viewer/window.ts`, a class with no React in it: `LogWindow`. It owns a `Lines`
(§6) and this state:

```ts
type WindowState = {
  landed: boolean;
  failed: boolean;
  /** Whether the viewport keeps the end in view. The user's hand changes it. */
  pinned: boolean;
  paused: boolean;
  /** The user's last scroll reached the bottom while not following: pin when the page it asked for finds no more. */
  wantsPin: boolean;
  /** The server's word on each edge, off the last page that way. */
  before: EdgeState;
  after: EdgeState;
  /** Where the after edge continues: the last page's `after.cursor`, or the last bookmark. Null before a landing. */
  resume: Cursor | null;
  /** The page in flight toward each edge, by ticket. */
  inFlight: { before: number | null; after: number | null };
  skipped: string[];
};

type EdgeState = { more: boolean; empty: boolean };
```

Three derived values, each a method:

- `following()`: `landed && pinned && !paused`. The adapter opens the follow exactly while
  it is true, at `resume`.
- `edges()`: what the adapter pages by and the render function draws. `before` as the server
  said. `after` is `{ more: false, empty: false }` while following, and otherwise
  `{ more: landed, empty: after.empty }`: a window that is not following is behind the live
  end whatever the last page said, since lines go on being written past it. This one rule
  replaces 1B's `Behind`, its `afterEdge` ternary and its paused-while-pinned arm.
- `atEnd()`: `following() && the last line is on screen`, the second from the adapter's
  range.

**Events**, each a method that changes the state, notifies the listener, and answers an
`Effect` for the adapter's next commit:

```ts
type Effect = { kind: 'none' } | { kind: 'hold' } | { kind: 'toEnd' } | { kind: 'toKey'; key: number };
```

- `landed(page, anchor)`: `lines.reset` with key 0 at the first line for `Head`, the last for
  `Tail`, and for `At` at index `min(page.anchor, lines.length - 1)`; `before` and `after` off
  the page; `resume = page.after.cursor`; `landed = true`; `pinned = pinToEnd && !page.after.more`,
  since a follow would read all the rest of a landing short of the end. Effect: `toEnd` for
  `Tail`, `toKey(0)` for `At`, `none` for `Head`.
- `landingFailed()`: `failed = true`, `pinned = false`. `jumpTo` lands again.
- `startPage(edge)`: a fresh ticket when `landed`, nothing is in flight that way, and
  `edges()[edge].more`; else null. The adapter fetches with it:
  `{ mode: 'Tail', cursor: before.cursor }` for `before`, `{ mode: 'Head', cursor: resume }`
  for `after`.
- `paged(ticket, page)`: ignored when the ticket is not the one in flight, which is how a page
  after asked before a follow opened is dropped: opening the follow clears `inFlight.after`.
  For `before`: effect `hold`, then `lines.prepend`, `before = page.before`, with `empty` set
  when the page added no line and said more. For `after`: `lines.append`, `after = page.after`,
  `resume = page.after.cursor`, the same `empty` rule, and when `wantsPin` and not
  `page.after.more`, `pinned = true` with effect `toEnd`. `skipped` off every page.
- `pageFailed(ticket)`: clears the ticket. The adapter reports the error.
- `followed(frames)`: lines appended less those held, each bookmark moving `resume`. Effect
  `toEnd`.
- `scrolledUp()`: `wantsPin = false`, `pinned = false`. The follow closes by derivation.
- `reachedBottom()`: nothing while following; else `wantsPin = true`. The adapter then pages
  after, since `edges().after.more` is true, and the page pins when it finds no more.
- `setPaused(paused)`.

`subscribe` and `getSnapshot` publish `CoreState` as 1B's handle does, so the core's `store.ts`
stays as the one store the handle serves across landings. A window is made per landing, as
`Landing` is keyed today.

### 6. The lines

`lines.ts` keeps the deque and the drop-by-cursor and loses `resume`, `from`, `resumeFrom` and
`PageFrom`: the window holds the position. Its callbacks go too; the window notifies.

### 7. The adapter

`core.tsx` keeps what touches the DOM and nothing else: `useVirtualizer`, `useOwnScroll`,
`useKeepPlace`, the scroll and resize listeners, `onNextFrame`, and `LogViewerCore` keying one
`Landing` per landing. `Landing` makes the window, lands through `client.fetch`, pages when the
visible range nears an edge and `startPage` answers a ticket, batches follow frames per
animation frame into `followed`, performs the window's effect in a layout effect after each
commit, and keeps the end in view on every commit while following. The landing's scroll is the
`toEnd` or `toKey` effect, so `before-paint.ts` goes. `moment.ts` goes with the overlap.

A page toward an edge is asked for once the range moves, never because an edge changed, and
not again until a frame after the last one landed, as today.

The props, `CoreRows`, `CoreHandle` and `CoreState` are 1B's. `REOPEN_OVERLAP_MS` and
`HIDDEN_FLUSH_MS` are no longer exported; the second stays a constant.

### 8. The fake client

`fake-client.ts` implements the new `Client`: cursors in the sidecar's shape (a line's as the
object, a page's edge as the object with an `s` the fake writes and ignores), the live-end rule
of §1 with a 5 s overlap, and a bookmark after each `append`. Its knobs are 1B's.

## Decisions this step asks for

- **The sidecar owns the position.** A page's cursor is where the next page continues, and the
  core never continues from a line. The inclusive pair goes from the wire, since only the
  `Around` read needs one and it is the sidecar's own.
- **The core mints no moment.** A clock the cluster does not share cannot say where the log's
  end is. The live-end cursor and the bookmark are the sidecar's, with its overlap.
- **A window that is not following is behind.** One rule for unpinned, paused and landed
  short of the end, at the cost of one small page when the user scrolls back to the bottom.
- **Hints ride the edge cursor, not the line's.** A line's cursor is sent 500 times a page; an
  edge's twice.
- **A window class, not a reducer.** The lines are a mutable deque and the effects are answers
  to calls; a class says that plainly.
- **Keep the scroll attribution.** It is the tested part of 1B that this step does not need to
  change.

## Tasks

In order. Each sidecar task is a pull request into `wip/log-views`; the webview task is one
pull request into `wip/log-views` replacing #228 and #229, with one commit per layer in the
order listed. Every pull request passes the verification below at its head.

1. **S1, the page's edges.** `services/logs/fetch.go`: `FetchQuery{Mode, Cursor *string, Limit}`,
   `FetchAround`, the `Around` double read. `services/logs/merge.go`: `Page{Records, Before,
   After PageEdge, Anchor *int, Skipped}`, `PageEdge{Cursor string, More bool}`, the edge rules of
   §1, `followOverlap` shared with S2. `graph/schema.graphqls` and `gqlgen.yml` (`LogsPageEdge`
   onto `logs.PageEdge`, `Around` onto `logs.FetchAround`), regenerate, `graph/schema.resolvers.go`.
   `docs/specs/log-viewer/1A-lines-from-the-sidecar.md`: the wire in §1, *Cursors and the
   second*, and the decision *One query with four cursor arguments*. **Planned.**
2. **S2, bookmarks.** `services/logs/follow.go`: `FollowBookmark`, `FollowFrame.Cursor`,
   `followBookmarkEvery`, the timer. Schema and `gqlgen.yml` for the member, regenerate.
   **Planned.**
3. **S3, the cursor and the hint.** `services/logs/cursor.go`: the encoding, `streamHint`,
   `parseCursor` answering both. `services/logs/fetcher.go`: `readOpts.hint`, `readBackward` and
   `backwardUntil` reading it, `firstTimestamp` skipped on a hint. `services/logs/merge.go`:
   the edges carry hints. **Planned.**
4. **W1, the webview.** On `log-viewer-1b-rework`, over the squashed commit:
   1. *Contract*: `src/lib/log-viewer/types.ts`, `README.md`, `fake-client.ts` and its test.
   2. *Window*: `lines.ts` trimmed, `window.ts`, `window.test.ts` with the walk, `lines.test.ts`.
   3. *Adapter*: `core.tsx`, `core.test.tsx`, `index.ts`; delete `moment.ts`, `before-paint.ts`
      and their tests.
   4. *Docs*: root `CLAUDE.md`'s log-viewer paragraph, `1B-the-headless-core.md`'s §3 and
      *Tests*, this spec's tasks. **Planned.**
5. **Step 2's spec.** `2-the-viewer-in-the-sidebar.md` §1: the client is one `fetch` and one
   `follow`; on a reconnect it reopens the follow from the last bookmark it saw rather than
   letting `subscribe-exchange` replay the opening cursor. **Planned.**

## Tests

Beside the files they cover. The cases 1B's `core.test.tsx` holds are the acceptance cases
here: each moves to `window.test.ts` when it needs no DOM, and stays when it does.

- `services/logs/fetch_test.go`: each row of the edge table in §1, the live-end moment less the
  overlap, `Around`'s `anchor` with lines on both sides, on one side, on none; a page cut by a
  stop answers the stop as its cursor; `TestPagesJoinWithoutLossOrRepeat` pages by `before.cursor`
  and `after.cursor` alone and still holds.
- `services/logs/follow_test.go`: a bookmark every interval, monotonic, never before the opening
  cursor; a follow opened after a bookmark re-sends at most the overlap; a bookmark is not sent
  after `Ended`.
- `services/logs/cursor_test.go`: a line's cursor decodes to its position and back; a moment is
  a bare time in and the object with `t` alone out; `v` other than 1, a tab-separated cursor,
  base64 of text that is not the object, an empty `c` and a negative `n` are each refused with
  `errBadCursor`; two cursors of one moment order by stream and ordinal as their positions do;
  a hint round-trips; an unknown key is ignored; a missing `s` is no hint.
- `services/logs/fetcher_test.go`: a hint's `first` skips the first-line GET; a hint's `spl`
  sizes the first reach; a `first` earlier than the real first line and an `spl` off by ten
  each read every line in range, counting the GETs.
- `graph/schema.resolvers_test.go`: `logsFetch` with each mode, reading the edges off the real
  `cursorOf`; a `Bookmark` frame on `logsFollow`.
- `window.test.ts`: every landing kind with the key 0 rule and its effect; a pinned landing short
  of the end lands unpinned; the ticket rule, a stale page after dropped; `edges()` while
  following, unpinned, paused, and failed; a page before adding nothing keeps its cursor and
  says empty; `wantsPin` set by `reachedBottom`, cleared by `scrolledUp`, honoured by the page
  that finds no more and not by one that finds more; a bookmark moves `resume` and a line does
  not; `following()` flips with pin and pause; `atEnd()`. The seeded walk from 1B, over a
  scroller the test models as a scroll top, a view height and a row height, with its
  invariants: one stretch of the log in order, following only at the end, pinned only by the
  user's hand, a failure reported once, the reader's row kept.
- `core.test.tsx`: the landing effects reach the scroller; a page before holds the first visible
  row; the follow is open exactly while following and opens at `resume`; follow frames flush
  per frame and on the hidden timer; a scroll up unpins between batches, coalesced with a batch,
  and before its event arrives; a clamp at the bottom does not pin; `measure` keeps the row or
  the end; `jumpTo` and a new client land again. The scripted client answers `Page`s.
- `fake-client.test.ts`: the live-end cursor, a bookmark after `append`, a cursor of its own
  refused when it is not.

## Security

**Widens:** nothing. A cursor is text the webview passes back unread, as today; `parseCursor`
refuses what it refused with base64 and JSON decoding in front, and the hint's fields reach the
API server only as a `sinceTime` and a `tailLines`, each bounded as the fetch's own values are.
A hint cannot make a read exceed the fetch's budget or window cap. The webview's core still talks
to nothing.

**Residual:** none new.

## When it lands

- Root `CLAUDE.md`, the log-viewer paragraph: the contract is one `fetch` and one `follow`,
  the core is a window and an adapter, no moment is minted, the walk runs on the window.
- `sidecar/CLAUDE.md`, *A cursor names a line*: the encoding, the hint, the edges and the
  bookmark.
- `docs/specs/log-viewer/README.md`: this row goes; *Fixed so far* says a page answers both
  edges and the sidecar mints every cursor.
- `1B-the-headless-core.md` stays until step 2 lands, amended as task 4 says.
- An ADR, *the sidecar owns the paging position*, recording why the inclusive pair and the
  webview's clock left the contract.

## Verification

Per [Verification commands](../README.md#verification-commands): `bash scripts/sandbox-dev-setup.sh`
first. Sidecar tasks: `cd sidecar && go test ./internal/services/logs ./graph/...`, then
`make lint-go`, `make vet-go`, and after a schema change the wire checks. Webview task:
`make test-changed` while working, then `pnpm codegen`, `pnpm build`, `make test-js`,
`make lint-js`.

By hand, with the sidecar running against a cluster whose deployment has been logging for a
day: a `Tail` `logsFetch` of 100 lines, then `Tail` with its `before.cursor`, repeated ten
times, each answering within a second and no line repeated or skipped across the pages; a
`logsFollow` opened `after` the first page's `after.cursor` sending a `Bookmark` within two
seconds.
