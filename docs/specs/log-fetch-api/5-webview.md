# Step 5 — The webview

**Files:** `src/lib/logs-client.ts`, `src/lib/inline-log-lines.ts`, `src/lib/log-viewer/`
(`types.ts`, `window.ts`, `core.tsx`, `fake-client.ts`, `index.ts`, `README.md`),
`src/components/widgets/log-viewer/` (`row.tsx`, `log-viewer.tsx`, `index.ts`),
`src/components/widgets/logs-view-inline.tsx`, `logs-read-card.tsx`, `logs-watch-card.tsx`,
`src/lib/chats.tsx`.
**Prerequisites:** steps 3 and 4.
**Status:** Planned.

## Goal

The app reads logs over the new wire with no change to how the viewer behaves. The viewer and
the transcript's two cards draw a cut line, and the `LogsRead` card marks the lines outside the
range. `pnpm build` is green again at the end of this step.

Read first: `src/CLAUDE.md`'s log viewer and tool-call paragraphs, `src/lib/log-viewer/README.md`
and `src/lib/logs-client.ts`. The wire is step 3's schema, generated into `src/gql/`.

## The client

`createLogsClient` implements the library's `Client` over `LogsFetchInput` and
`LogsFollowInput`. It builds the scope once from the `LogsViewAction` through `scopeInput`,
today's `viewInput` renamed, and sends it on every first page, which is what the window needs.
`fetch` is implemented for the first time: today it rejects. The query is a union, so the
mapping is one arm per kind:

| Library query | Wire request |
| --- | --- |
| `First` | `scope`, `pageFrom: First`, bounds open |
| `Last` | `scope`, `pageFrom: Last`, bounds open |
| `At` | two pages at once: `scope`, `since: at`, `pageFrom: First` and `scope`, `until: at - 1 ns`, `pageFrom: Last` |
| `Continue` | `cursor` and `seen` as they came, nothing else |

`limit` is sent as `pageSize`; `maxMessageBytes` and `maxPageBytes` are left at 0, the server's
caps. The page's three strings and its `notices` pass through as they came. **Every fetch,
both `At` halves included, runs with `requestPolicy: 'network-only'`.** The app's urql client
caches queries by their variables, and a page's variables repeat: a `Last` landing read again,
or a `follow` cursor polled again, must reach the sidecar and not answer the page it answered
before.

**`At` is two disjoint pages joined.** The `Last` half ends the nanosecond before `at`, so
every line at `at` is in the `First` half and none in both. The client sends both, waits for
both, and answers one `Page`: the `Last` half's lines then the `First` half's, oldest first;
`prev` is the `Last` half's and `next` and `follow` the `First` half's; `anchor` is the `Last`
half's length. The halves must not overlap: a page limit cuts each at its own end, so with
more lines at `at` than `limit` an overlapping `Last` half would hold the newest of them and
the `First` half the oldest, out of order and with a hole between. `previousNanosecond` in
`src/lib/log-viewer/lines.ts`, beside `compareTimestamps` and exported from the index, steps a
stamp back one nanosecond, borrowing a second when the fraction is zero. One half failing fails
the `At`.

**A cursor is the server's, as it came.** The library's `Cursor` stays an opaque string the
client passes through: a page cursor holds the window, so the client wraps nothing and sends no
scope or bounds on a `Continue`. A line's cursor is the record's, and the client never sends one
as `cursor`: it travels only in `seen`. A cursor never expires and survives a cache clear. A
continuation the server refuses arrives as a validation error whose `rule` is `cursor-invalid`,
which happens only across a version of Kstack, and a running session crosses none: the webview
and the sidecar ship in one bundle. The client surfaces it as the fetch's error, and the window
does with it what it does with any failed page, waits for the user's next scroll. No reset path
is built for it.

**`follow(cursor, seen)` sends both through as they came,** on `LogsFollowInput` with
`maxMessageBytes: 0` and no scope: the cursor is a page's `next` or `follow`, which the server
reads as that page's poll and then goes on from. The viewer never sends `since` to open a
follow; a follow from a moment is a page over that window and then its cursor. **A reconnect sends
the same `cursor` and the client's own `seen`**, through `reconnectVariables`: the `seen` it
was given, as it came, followed by the newest line it delivered of each container, keyed by
the frame's `stream`. The given entries are opaque, so the client cannot tell which one a
delivered line replaces; it sends both, and the server keeps the newer of two for one
container whatever their order. The window sees no reconnect, so it holds nothing for one. The frames map by `type`: `Line` to the library's `Line` with
`lineOf`; `Notice` to `Notice`; `Ended` to `Ended` with the notice's `message` as the reason,
the code dropped. A follow the server refuses to open calls `onFailed` as today, and the viewer
reopens from a page.

A page from `next` or `follow` may hold a line stamped older than one the window holds, from a
stream whose clock runs behind. The library appends a page after what it holds and prepends a
page before it, in page order, and drops a repeat by cursor; a straggler is drawn where it
lands, as a follow's out-of-order line is today. Nothing in the library changes for it. The
README's edges section says so in one sentence.

The two documents are step 3's, written there so its codegen passes: `LogsFetch` selects
`truncatedBytes` on each record, `page { prev next follow }` and `notices { code message }`,
and not `request`, since the viewer keeps its own; `LogsFollow` selects `type`,
`record { … truncatedBytes }`, `notice { code message }` and `ended { code message }`. This
step changes them only if the code needs a field they lack.

## The library

The contract in `types.ts` becomes:

```ts
/** One read. `First` and `Last` read from an end, `At` reads `limit` lines either side of a moment, and `Continue` reads on from a page's edge. */
export type FetchQuery =
  | { kind: 'First'; limit: number }
  | { kind: 'Last'; limit: number }
  | { kind: 'At'; at: string; limit: number }
  | { kind: 'Continue'; cursor: Cursor; seen: Cursor[] | null; limit: number };

export type Notice = { code: string; message: string };

export type Page = {
  /** Oldest first, as the client sorted them, whatever the query. */
  lines: LogLine[];
  /** Where a page that way goes on, null with nothing that way. `follow` is the live end: set on a page that read every stream to its end. */
  edges: { prev: Cursor | null; next: Cursor | null; follow: Cursor | null };
  /** For `At`: the index of the first line at or after the moment, `lines.length` with none. Null otherwise. */
  anchor: number | null;
  /** What the page could not do, one per notice, for the viewer to show. */
  notices: Notice[];
};

export type FollowFrame =
  | { kind: 'Line'; line: LogLine }
  | { kind: 'Notice'; notice: Notice }
  | { kind: 'Ended'; reason: string };

export type Client = {
  fetch: (query: FetchQuery) => Promise<Page>;
  follow: (cursor: Cursor, seen: Cursor[], onFrame: (frame: FollowFrame) => void, onFailed: (error: unknown) => void) => () => void;
};
```

`index.ts` exports `Notice` and drops `FetchMode` and `PageEdge`. `LogLine` gains:

```ts
  /** The bytes the server cut off `message` at the request's cap, 0 when whole. */
  truncatedBytes: number;
```

`Anchor` stays `Head`, `Tail` or `At`: a landing is `First`, `Last` or `At`, and every page
after it a `Continue`. **The window reads an edge off its strings, and its rules stay.** The
before edge has more while `prev` is set. The after edge's `more` is derived, as today: a
window that is not following is behind, whatever its last page said, so it always has a page
after, and that page is a `Continue` from `resume`, which is `next ?? follow`. That is the
poll of the live end: a window unpinned by a scroll, or whose follow was refused, scrolls to
the bottom and reads on from `follow`, and pins when the page it gets says `next` is null,
since a follow would have brought nothing more. `empty` and a page of repeats read `next`
alone: an empty poll of the live end is not a spent budget, and a poll of repeats does not
page again at once. The `At` join lives in the client, so the window still lands one page and
reads `anchor` off it.

**A notice is kept once per code and message.** A follow sends `stream-skipped` once per
container generation and every page may repeat a page's notice, so the list would grow as the
user scrolls or waits. The window joins a page's notices and a `Notice` frame the one way,
dropping a repeat. A container skipped again after a later restart with the same message
therefore shows once; the notice says the stream was skipped, not how many times.

**The window keeps the newest line per stream, not a checkpoint.** `LogWindow` holds a map of
container to the newest line's `cursor` as lines land, from pages and from frames, keyed as the
server keys `seen`: namespace, pod and container, the current and previous runs under one key.
`resume` stays the follow's cursor, the after edge of the page that pinned the window, and
never moves on a frame. **Every `Continue` from the after edge sends `seen` from the map**,
followed or not, so no flag marks the page after a follow: the server keeps the newer of a
`seen` entry and the cursor's own, so a repeat is harmless, and it refuses `seen` only with
`Last`, a `prev`, padding or a counted `after`, none of which the viewer sends. A `Continue`
from the before edge sends `seen: null`.
`notices` replaces `skipped` in the window's `CoreState`, which `core.tsx` carries and
`log-viewer.tsx` draws.

`lineOf` in `logs-client.ts` fills `truncatedBytes`. `FakeClient` answers the four kinds and
fills `truncatedBytes` (`0`); it gains a `truncate` option that sets it on chosen lines, for the
row test; its `skip` option becomes a notice with a code; it honours `seen` by reading each
named stream past its line and the rest from the cursor, as the server does, and sends no
checkpoint. The library draws nothing, as today.

Rewrite `README.md`'s *The contract* and *The edges* sections to the union and the three
strings: the edges table is keyed by query kind, every `Continue` from the after edge sends
`seen`, a `Notice` frame shows as a page's notice does, and a page going on from the after edge
may hold a line older than one held, drawn where it lands.

## The inline window

`src/lib/inline-log-lines.ts` is the contract's other reader: it feeds the `LogsView` card's
eight-line window. It moves to the same shapes and behaves as before:

- `inlinePlan`'s first page is `First`, `Last`, or `At` with the moment, for the `Head`,
  `Tail` and `At` anchors and the frozen tail. A frozen tail keeps the lines before the
  anchor, which with disjoint halves is the `Last` half alone, so more lines at the moment than
  the limit stay out of it. Its test covers that case.
- `readInline` reads on with a `Continue` from `prev` for a tail and from `next` for the
  others, `seen: null`, while that string is set and the window is short.
- The follow starts from the first page's `next ?? follow` and sends `seen: []`, since it
  goes on from a page's own cursor.

`inline-log-lines.test.ts` moves with it.

## What is drawn

- **The cut marker** is `CutNotice` in `row.tsx`, exported from
  `components/widgets/log-viewer/index.ts` beside `Dot` and `GapNotice`, since nothing outside
  the viewer imports one of its parts. A cut line ends with it: a muted ` … 18 KiB cut` showing
  `truncatedBytes` in KiB (one decimal, `B` under 1 KiB), with `title="Cut by the server"`.
  Nothing opens the whole line yet; that is a later change.
- **The viewer's row** (`row.tsx`) draws `CutNotice` after the message.
- **The transcript's row** (`LineRow` in `logs-view-inline.tsx`) draws it too, since both
  cards draw `LineRow`. It gains a `tag: 'before' | 'after' | null` prop: a tagged line is
  muted with the tag before its timestamp.
- **The `LogsRead` card** (`logs-read-card.tsx`): `logLine` carries `truncatedBytes`, so
  `LineRow` draws the cut. The card tags a line `before` when it is stamped before the action's
  `since`, and `after` when it is stamped after the range's end: `until` when set, else the
  nanosecond before `resolvedAt` when `after` is set, since an open `until` with padding ends
  the range there (`services/logs/README.md`, guarantee 2). With `until` open and no `after`,
  nothing is tagged `after`. The stamps may differ in fraction digits, so each comparison is
  `compareTimestamps`, which the library's index exports. `rangeLine` draws the padding
  (`100 lines before`, `PT5M after`, the span as the action holds it) and `pageFrom` where
  `mode` was drawn, with the card's words as they are: `First` is *oldest first* and `Last`
  *the newest*; nothing on a continuation. `Beyond` is built from `olderMore`, `newerMore`
  and `notices`: *Older lines exist.*, *Newer lines exist.*, then each notice's `message` with
  its `code` in a `title`. Every string is cluster text through `VisibleText`, as today.
- **The viewer's notices** (`log-viewer.tsx`): drawn where `skipped` lines are today, `message`
  shown and `code` in a `title`.
- **The `LogsWatch` card** (`logs-watch-card.tsx`): its held lines go through the same
  `logLine`, so a cut line draws its cut. It tags nothing.
- **`chats.tsx`**'s messages document already selects the new action fields (step 3).
  `logs-view.tsx` only re-exports the action's type.

## Tasks

1. **Library types.** The union, `Notice`, the three strings, `truncatedBytes` and the follow's
   signature in `types.ts`; the window and the core over them, with `notices` for `skipped`;
   the per-container map and `seen` on every `Continue` from the after edge; `FakeClient`; every test
   fixture that builds a `LogLine`, a `FetchQuery` or a `Page` (`grep -rln "gap: null" src`
   finds the first). Tests: each landing kind sends its query and each page after it a
   `Continue`; the before edge's `more` and the after edge's `empty`, read-on and pin read
   `prev` or `next` alone, so an empty poll of the live end neither spends the budget nor pages
   again; a window with `next` null and `follow` set, unpinned by a scroll or by a refused
   follow, reads on from `follow` and pins on a page with `next` null; a repeated notice is held once; a `Continue` from the after edge sends the newest line per
   container, and after a follow holds no line twice; a `Notice` frame reaches `notices`.
2. **Client.** The `fetch` mapping with the `At` join, the `follow` mapping, and `scopeInput`,
   over step 3's documents. Tests in `logs-client.test.ts` for each row of the table, for the
   three strings and the notices passed through, for a repeated query reaching the server after
   its answer changed, for a `Continue` sent as the cursor with
   `seen`, for a refused cursor reaching the caller, for `At`: the `Last` half's bound one
   nanosecond before `at`, the join's order, the anchor with and without a `Last` half, more
   lines at `at` than `limit` landing in order with no line held twice, and one half failing;
   and for `follow`: a cursor and `seen` sent through, a reconnect sending the `seen` given
   followed by the newest delivered line per container, each frame type mapped, and a refused
   open. `previousNanosecond`'s test in `lines.test.ts` covers the borrowed second and a
   stamp with fewer than nine digits.
3. **Inline window.** `inline-log-lines.ts` on the new shapes and its tests.
4. **Rows.** `CutNotice`, its export, the viewer's row and `LineRow` with its `tag`, and their
   tests.
5. **Cards and notices.** `logs-read-card.tsx` and its tests, one per tag rule; the watch
   card's test for a cut line; the viewer's notices.
6. **Build and docs.** `pnpm build`, `make test-js`, `make lint-js`. Rewrite
   `src/lib/log-viewer/README.md`'s contract and edges sections and the log viewer and
   `LogsRead` sentences in `src/CLAUDE.md`.

## Acceptance

- `pnpm build`, `make test-js` and `make lint-js` pass.
- The viewer behaves as before over the new wire: `log-viewer.test.tsx` passes with no change
  but its fixtures' shapes and the follow's signature.
- `make cover-js` does not drop below `scripts/coverage-threshold`.

## When it lands

`src/CLAUDE.md`'s log viewer and `LogsRead` paragraphs and `src/lib/log-viewer/README.md`
describe the query union, the three strings and `seen`.
