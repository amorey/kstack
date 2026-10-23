// Copyright 2026 The Kstack Authors
// Copyright 2024 The Kubetail Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { compareTimestamps, containerKey } from './lines';
import type { Client, Cursor, FetchQuery, FollowFrame, LogLine, Notice, Page } from './types';

// A `Client` over a log it invents, for the tests. It models what that wire does to a viewer — edges and repeats, a follow whose
// streams interleave, `seen`, latency, failures, a stream it could not read — so a viewer that
// holds against it holds against the sidecar.

export type FakeStream = {
  stream: LogLine['stream'];
  place: LogLine['place'];
  /** Turns the stream takes in each round of the rotation. 1 by default. */
  weight?: number;
};

export type FakeClientOptions = {
  /** How many lines the log holds when the fake is made. */
  lines?: number;
  /** How far apart those lines are stamped, the last at the clock's time. 1 by default. */
  spacingMs?: number;
  /** Who writes the log. */
  streams?: FakeStream[];
  /** Lines written per second, on a timer, until `close()`. */
  rate?: number;
  /** How long every fetch, and a follow's replay, takes. */
  delayMs?: number;
  /** How far before the newest line the live end sits. */
  repeat?: number;
  /** The hole each line carries, by its index. None by default. */
  gaps?: Record<number, NonNullable<LogLine['gap']>>;
  /** The bytes cut off each line, by its index. None by default. */
  truncate?: Record<number, number>;
};

class Follower {
  /** Whether the replay went out, after which each append sends its own lines. */
  live = false;

  /** False once cancelled or ended: it sends nothing after. */
  open = true;

  timer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private readonly onFrame: (frame: FollowFrame) => void,
    /** Whether the follow sends the line at an index. */
    readonly wants: (index: number) => boolean,
  ) {}

  send(frame: FollowFrame): void {
    if (this.open) this.onFrame(frame);
  }

  close(): void {
    this.open = false;
    clearTimeout(this.timer);
  }
}

const DEFAULT_STREAM: FakeStream = {
  stream: { namespace: 'default', pod: 'fake-0', container: 'main', previous: false },
  place: { node: 'fake-node', region: '', zone: '', os: 'linux', arch: 'amd64' },
};

// Each stream takes `weight` turns a round, spread evenly through it: turn k of a stream
// sits at (k + 0.5) / weight of the way round.
function rotation(streams: FakeStream[]): number[] {
  const turns: { at: number; stream: number }[] = [];
  streams.forEach((s, stream) => {
    const weight = s.weight ?? 1;
    for (let k = 0; k < weight; k += 1) turns.push({ at: (k + 0.5) / weight, stream });
  });
  turns.sort((a, b) => a.at - b.at || a.stream - b.stream);
  return turns.map((turn) => turn.stream);
}

export class FakeClient implements Client {
  private readonly log: LogLine[] = [];

  private readonly streams: FakeStream[];

  private readonly turns: number[];

  private readonly delayMs: number;

  private readonly repeat: number;

  private readonly gaps: Record<number, NonNullable<LogLine['gap']>>;

  private readonly truncate: Record<number, number>;

  private lastMs = -Infinity;

  private readonly followers = new Set<Follower>();

  private readonly skipped = new Map<string, Notice>();

  private readonly passed: { from: number; to: number }[] = [];

  private failFetch = false;

  private failFollow = false;

  private emptyPending = false;

  private timer: ReturnType<typeof setInterval> | undefined;

  constructor(options: FakeClientOptions = {}) {
    this.streams = options.streams?.length ? options.streams : [DEFAULT_STREAM];
    this.turns = rotation(this.streams);
    this.delayMs = options.delayMs ?? 0;
    this.repeat = options.repeat ?? 20;
    this.gaps = options.gaps ?? {};
    this.truncate = options.truncate ?? {};
    const count = options.lines ?? 1000;
    const spacing = options.spacingMs ?? 1;
    const now = Date.now();
    for (let i = 0; i < count; i += 1) this.addLine(now - (count - 1 - i) * spacing);
    const rate = options.rate ?? 0;
    if (rate > 0) this.timer = setInterval(() => this.append(1), 1000 / rate);
  }

  /** Writes `count` lines stamped with the clock's time. */
  append(count: number): void {
    const from = this.log.length;
    for (let i = 0; i < count; i += 1) this.addLine(Date.now());
    this.followers.forEach((follower) => {
      if (follower.live) this.send(follower, from, this.log.length);
    });
  }

  /** Stops the growth `rate` started. */
  close(): void {
    clearInterval(this.timer);
  }

  async fetch(query: FetchQuery): Promise<Page> {
    const refused = this.refuse(query);
    if (refused) throw new Error(refused);
    const failing = this.failFetch;
    this.failFetch = false;
    // Read at the call, as a request is answered from the log as it was when it arrived.
    const page = failing ? null : this.read(query);
    await delay(this.delayMs);
    if (page === null) throw new Error('fake: the fetch failed');
    return page;
  }

  follow(cursor: Cursor, seen: Cursor[], onFrame: (frame: FollowFrame) => void): () => void {
    const parsed = this.writtenPage(cursor);
    if (parsed === null || parsed.kind === 'prev') throw new Error(`fake: not a next or follow it wrote: ${cursor}`);
    const refused = this.refuseSeen(seen);
    if (refused !== null) throw new Error(refused);
    if (this.failFollow) {
      this.failFollow = false;
      throw new Error('fake: the follow failed');
    }
    const start = parsed.index;
    const follower = new Follower(onFrame, this.reader(start, seen));
    this.followers.add(follower);
    // What is appended before the replay is held back and sent with it.
    follower.timer = setTimeout(() => {
      follower.live = true;
      this.send(follower, start, this.log.length);
    }, this.delayMs);
    return () => {
      follower.close();
      this.followers.delete(follower);
    };
  }

  /** Makes the next fetch reject, or the next follow throw. */
  failNext(what: 'fetch' | 'follow'): void {
    if (what === 'fetch') this.failFetch = true;
    else this.failFollow = true;
  }

  /** Makes the next fetch answer no line, as a read that spent its budget does. */
  emptyNext(): void {
    this.emptyPending = true;
  }

  /** Keeps a stream's lines out of every page and follow, and names it in each page's notices. */
  skip(stream: LogLine['stream']): void {
    const { namespace, pod, container, previous } = stream;
    const name = `${namespace}/${pod}/${container}${previous ? ' (previous)' : ''}`;
    this.skipped.set(streamKey(stream), { code: 'stream-skipped', message: `${name}: the fake skipped it` });
  }

  unskip(stream: LogLine['stream']): void {
    this.skipped.delete(streamKey(stream));
  }

  /** Sends `Ended` to every open follow, and nothing after it. */
  endFollows(reason: string): void {
    this.followers.forEach((follower) => {
      follower.send({ kind: 'Ended', reason });
      follower.close();
    });
    this.followers.clear();
  }

  /** Each span an empty read moved its edge across, whose lines no read will serve. */
  passedOver(): { from: number; to: number }[] {
    return [...this.passed];
  }

  get lineCount(): number {
    return this.log.length;
  }

  lineAt(index: number): LogLine {
    return this.log[index];
  }

  // The window must send only what it was given, so the fake refuses anything else.
  private refuse(query: FetchQuery): string | null {
    if (query.limit < 1) return `fake: a limit of ${query.limit}`;
    if (query.kind !== 'Continue') return null;
    const parsed = this.writtenPage(query.cursor);
    if (parsed === null) return `fake: not a page cursor it wrote: ${query.cursor}`;
    if (query.seen === null) return null;
    if (parsed.kind === 'prev') return 'fake: `seen` beside a `prev`';
    return this.refuseSeen(query.seen);
  }

  private refuseSeen(seen: Cursor[]): string | null {
    const unwritten = seen.find((cursor) => {
      const parsed = parse(cursor);
      return parsed === null || parsed.kind !== 'line' || parsed.index >= this.log.length;
    });
    return unwritten === undefined ? null : `fake: a seen line it did not write: ${unwritten}`;
  }

  /** A page cursor it wrote, parsed; null for any other string. */
  private writtenPage(cursor: Cursor): ParsedCursor | null {
    const parsed = parse(cursor);
    return parsed !== null && parsed.kind !== 'line' && parsed.index <= this.log.length ? parsed : null;
  }

  // Whether a read on from position `start` takes the line at an index: each container named in
  // `seen` past the newer of its lines there, the rest from `start`.
  private reader(start: number, seen: Cursor[]): (index: number) => boolean {
    const past = new Map<string, number>();
    seen.forEach((cursor) => {
      const { index } = parse(cursor) as ParsedCursor;
      const key = containerKey(this.log[index].stream);
      past.set(key, Math.max(index, past.get(key) ?? -1));
    });
    return (index) => index >= start && index > (past.get(containerKey(this.log[index].stream)) ?? -1);
  }

  private read(query: FetchQuery): Page {
    const empty = this.emptyPending;
    this.emptyPending = false;
    const n = this.log.length;
    // The read starts at `start` and covers the lines it takes, `low` up to `high`: First and a
    // `next` or `follow` read on from the start, Last and a `prev` back from it, and At both ways.
    let start: number;
    let low: number;
    let high: number;
    let taken: number[];
    const { limit } = query;
    const continued = query.kind === 'Continue' ? (parse(query.cursor) as ParsedCursor) : null;
    if (query.kind === 'Last' || continued?.kind === 'prev') {
      start = continued === null ? n : continued.index;
      low = Math.max(start - limit, 0);
      high = start;
      taken = indexRange(low, high);
    } else if (query.kind === 'At') {
      start = this.firstAtOrAfter(query.at);
      low = Math.max(start - limit, 0);
      high = Math.min(start + limit, n);
      taken = indexRange(low, high);
    } else {
      start = continued === null ? 0 : continued.index;
      const wants = this.reader(start, query.kind === 'Continue' ? (query.seen ?? []) : []);
      const ahead = indexRange(start, n).filter(wants);
      taken = ahead.slice(0, limit);
      low = start;
      high = ahead.length > limit ? taken[limit - 1] + 1 : n;
    }

    // A read the limit cut short goes on from the cut. Any other is the live end, `repeat` lines
    // before the newest, so a page or a follow from it sends some again — never from before
    // where the read started, or, for an empty read, before what it passed over.
    const cut = high < n;
    const liveAt = Math.max(n - this.repeat, empty ? start : low);
    if (empty) {
      const to = cut ? high : liveAt;
      if (to > low) this.passed.push({ from: low, to });
    }
    const kept = (indexes: number[]) =>
      empty ? [] : indexes.map((i) => this.log[i]).filter((line) => !this.isSkipped(line));
    return {
      lines: kept(taken),
      edges: {
        prev: low > 0 ? pageCursor('prev', low) : null,
        next: cut ? pageCursor('next', high) : null,
        follow: cut ? null : pageCursor('follow', liveAt),
      },
      anchor: query.kind === 'At' ? kept(indexRange(low, start)).length : null,
      notices: [...this.skipped.values()],
    };
  }

  private isSkipped(line: LogLine): boolean {
    return this.skipped.has(streamKey(line.stream));
  }

  // The log is in time order, so a binary search finds it.
  private firstAtOrAfter(at: string): number {
    let low = 0;
    let high = this.log.length;
    while (low < high) {
      const mid = Math.floor((low + high) / 2);
      if (compareTimestamps(this.log[mid].timestamp, at) < 0) low = mid + 1;
      else high = mid;
    }
    return low;
  }

  // Sends the lines `from`..`to` the follow wants, one stream's batch after another, as the
  // sidecar's per-stream reads do.
  private send(follower: Follower, from: number, to: number): void {
    const lines = indexRange(from, to)
      .filter(follower.wants)
      .map((i) => this.log[i])
      .filter((line) => !this.isSkipped(line));
    this.streams.forEach(({ stream }) => {
      lines.filter((line) => line.stream === stream).forEach((line) => follower.send({ kind: 'Line', line }));
    });
  }

  private addLine(clock: number): void {
    const ms = Math.max(clock, this.lastMs);
    this.lastMs = ms;
    const index = this.log.length;
    const { stream, place } = this.streams[this.turns[index % this.turns.length]];
    this.log.push({
      timestamp: new Date(ms).toISOString(),
      cursor: lineCursor(index),
      message: `fake line ${index} from ${stream.pod}/${stream.container}`,
      truncatedBytes: this.truncate[index] ?? 0,
      stream,
      place,
      gap: this.gaps[index] ?? null,
    });
  }
}

// A line's cursor names its index; a page's names the gap before index `p`, so `p` runs
// 0..length, and the edge it came from. Each carries a prefix nothing else writes.
const lineCursor = (index: number): Cursor => `fake:line:${index}`;
const pageCursor = (edge: 'prev' | 'next' | 'follow', position: number): Cursor => `fake:${edge}:${position}`;

const CURSOR = /^fake:(line|prev|next|follow):(\d+)$/;

type ParsedCursor = { kind: 'line' | 'prev' | 'next' | 'follow'; index: number };

function parse(cursor: Cursor): ParsedCursor | null {
  const match = CURSOR.exec(cursor);
  return match ? { kind: match[1] as ParsedCursor['kind'], index: Number(match[2]) } : null;
}

const indexRange = (from: number, to: number) => Array.from({ length: Math.max(to - from, 0) }, (_, k) => from + k);

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

const streamKey = ({ namespace, pod, container, previous }: LogLine['stream']) =>
  `${namespace}/${pod}/${container}/${previous}`;
