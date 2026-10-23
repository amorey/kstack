# The log viewer's library

The headless part of the log viewer: what feeds it, the lines it holds, the window that
decides what every event does, and the core that puts the window behind a scroller. Nothing
here draws a row.

## The contract

`types.ts` is the seam between the window and whatever feeds it. A `Client` has two methods:

- `fetch(query)` answers a `Page`. The query is one of four kinds: `First` and `Last` read from
  an end, `At` reads `limit` lines either side of a moment, and `Continue` reads on from a page's
  edge, past each line in `seen` for its container. A page is its lines oldest first, its
  `edges` (`prev`, `next` and `follow`, each a cursor or null), and its `notices`, each a `code`
  and a `message`.
- `follow(cursor, seen, onFrame)` sends a view's lines as they are written, from a page's `next`
  or `follow`: `Line`, `Notice`, and `Ended`, the last frame.

**A cursor is opaque.** The client writes every one; the library reads none and passes each
back as it came. A line's cursor names it, and a line whose cursor is held is a repeat, dropped.
A page cursor names where the page that way goes on. A line's cursor travels only in `seen`.

The types mirror the wire's `LogRecord`, `LogRecordsPage`, `LogNotice` and `LogsFollowFrame`
field for field. `LogLine.truncatedBytes` is what the server cut off the message, 0 when whole.

**A line may carry a hole.** `LogLine.gap` is the stamps of the last known line before a range of
its stream that neither the cluster nor the log cache holds, and of the first known line past it;
null when none. The sidecar puts it on the first line past the hole that the view passes, so a repeat dropped by
cursor drops nothing new. The library holds it as part of the line and draws nothing of it.

### The edges

*The live end* is `follow`: set on a page that read every stream to its end, in place of `next`.
A page from it polls for lines written since, and a follow opens there.

| Query | `prev` | `next` | `follow` |
| --- | --- | --- | --- |
| `First` | null | set when a line lies newer | *the live end* when `next` is null |
| `Last` | set when a line lies older | null | *the live end* |
| `At` | as `Last`, from the moment back | as `First`, from the moment on | as `First` |
| `Continue` from `prev` | as `Last`, back from the cursor | ignored | ignored |
| `Continue` from `next` or `follow` | ignored | as `First`, on from the cursor | as `First` |

- **The window reads an edge off its strings.** The before edge has more while `prev` is set.
  After, a page with no line and a `next` is a read that spent its budget, read on by hand; an
  empty poll of the live end spent nothing.
- **Every `Continue` from the after edge sends `seen`**, the newest line held of each container,
  followed or not; one from the before edge sends `seen: null`. The server keeps the newer of a
  container's line and the cursor's own, so a line it already passed is harmless.
- A page or a follow that goes on from an edge misses no line. It may send again lines at or past
  it, which the window drops by cursor.
- A page going on from the after edge may hold a line stamped older than one held, from a stream
  whose clock runs behind. It is drawn where it lands, as a follow's out-of-order line is.
- A follow's lines arrive per stream, not merged: two containers' lines can interleave out of
  time order. A `Notice` frame shows as a page's notice does. Nothing comes after `Ended`.

## The window

`LogWindow` (`window.ts`) is a class with no React in it. It holds the lines (`lines.ts`, over
the keyed deque in `double-tailed-array.ts`), both edges, the pin, the pause, where the after
edge resumes (the last page's `next`, else its `follow`) and the newest line of each container.
Each event changes that state and answers an `Effect` for the scroller: `none`, `hold` (keep the
first visible row in place), `toEnd`, or `toKey`. It publishes a `CoreState`
through `subscribe` and `getSnapshot`, `useSyncExternalStore`'s shape.

Its rules, one each:

- **Keys are stable.** Key 0 is where the view landed, and a prepend lowers nothing held.
- **Only a `Tail` landing pins**, and only with `pinToEnd`. After that the pin is the user's
  hand: a scroll up or a pointer down on the lines unpins, and a scroll to the bottom pins once
  the page it asks for finds no more.
- **Following is derived**: landed, pinned, not paused, not ended. Frames that arrive while not
  following are dropped.
- **A window that is not following is behind**: its after edge always has more. A paused one
  asks for nothing after.
- **A page of repeats reads on** at once; a page with no line that has a page on is a spent
  budget the user reads on from by hand. Both read `prev` or `next` alone.
- **The follow opens at `resume`**, with `seen`, and a frame never moves `resume`: the page after
  a follow goes on from the page that pinned it, past every line the follow brought.
- **A notice is held once per code and message**, a page's and a `Notice` frame's alike. A
  landing starts the list over.
- **One page per edge is in flight**, named by a ticket. A stale ticket's page changes nothing.
  Each edge in the state reads `loading` while it has more and a page that way holds a ticket.
- **A refused follow unpins**, whether `follow` throws or calls `onFailed` later, so the next
  scroll to the bottom tries again through a page.

## The core

`LogViewerCore` (`core.tsx`) is a React component with a render function as its child. It
renders the scroller, a focusable `div` with its own overflow, and hands the render function
`CoreRows`: the rows on screen, each with its key, line and `start`, the sizer's `totalSize`,
both edges, and `loadMore` for an edge row's button. It decides nothing about lines; it tells
the window what the DOM and the client did and performs what the window answers.

- **One landing, one window.** Each landing — the mount, `jumpTo`, `retry`, a new `client` — is
  a fresh window, scroller and `@tanstack/react-virtual` virtualizer. Loading ends a frame
  after the landing settles.
- **Paging follows the visible range**, once loading has ended and while no re-measure is in
  flight. A page that landed holds its edge until the next frame; a page of repeats reads on.
- **The follow is open exactly while the window follows**, its frames handed over once per
  animation frame, or after 250 ms when no frame comes.
- **Intent comes from input events.** A scroll is the user's within 150 ms of a wheel or a
  scrolling key on the scroller, or while a pointer or a touch is down on it, and a pointer down
  unpins by itself. Only the pin is read off it.
- **Answers are state**, so each brings its own commit, performed in the layout effect after
  it: a hold keeps the first visible row by its key, `toEnd`, and `toKey` through
  `scrollToIndex`. While following, every commit that changes the sizer and every resize keeps
  the end in view.
- **The store** (`createCoreStore`) is where the core publishes the window's `CoreState`. A
  parent holds it and reads it with `useCoreFields`, which re-renders only when a field it names
  moves: the visible range moves the state on every row scrolled past. Each landing writes it
  before its first paint.
- **`scrollerRef`** gets each landing's scroller, a callback since each landing mounts a new
  one, for a caller that reads its size or its sideways scroll. It is the DOM node and nothing
  else: the core still knows no columns.

## The fake

`FakeClient` (`fake-client.ts`) is a `Client` over a log it invents, for the tests. It writes its history `spacingMs` apart (a
millisecond by default) from weighted streams, grows by `append` or at a `rate`, and models what
the wire does to a viewer: a live end `repeat` lines back, `seen` read per container, a follow that
sends per stream, latency (`delayMs`), holes (`gaps`) and cuts (`truncate`) on chosen lines, and
`failNext`, `emptyNext`, `skip` (a `stream-skipped` notice) and `endFollows`. `index.ts` leaves it out: import it by its file.

## The boundary

This directory imports `react`, `@tanstack/react-virtual` and its own files, and nothing else:
no `@/` import, and no relative import that leaves it. It knows lines, cursors and anchors, and nothing about columns, the viewport,
GraphQL or the error bus.
