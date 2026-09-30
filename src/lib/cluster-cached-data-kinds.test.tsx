// Copyright 2026 The Kstack Authors
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

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// Mocks ---------------------------------------------------------------

// Same seams as cluster-cached-data-events.test.tsx: an accumulator stand-in for
// `useWatchSubscription` that captures the reducer, the real `applyChange`, and the
// active-cluster join stubbed to name a cache.
const { useWatchSubscriptionMock, useActiveClusterMock } = vi.hoisted(() => ({
  useWatchSubscriptionMock: vi.fn(),
  useActiveClusterMock: vi.fn(),
}));

vi.mock('@/lib/graphql/use-watch-subscription', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/graphql/use-watch-subscription')>()),
  useWatchSubscription: useWatchSubscriptionMock,
}));
vi.mock('@/lib/clusters', () => ({
  applyChange: <T,>(items: Map<string, T>, type: string, id: string, entity: T) => {
    if (type === 'Deleted') items.delete(id);
    else items.set(id, entity);
  },
}));
vi.mock('@/lib/active-cluster', () => ({ useActiveCluster: useActiveClusterMock }));

const { useClusterCachedDataKinds } = await import('./cluster-cached-data-kinds');

// Helpers -------------------------------------------------------------

function kind(resource: string, over: Partial<{ apiVersion: string; count: number }> = {}) {
  return {
    apiVersion: over.apiVersion ?? 'apps/v1',
    kind: resource.replace(/s$/, ''),
    resource,
    scope: 'Namespaced',
    isCRD: false,
    count: over.count ?? 1,
    printerColumns: [],
  };
}

const resources = (kinds: { resource: string }[]) => kinds.map((k) => k.resource);

// urql accumulator stand-in — folds a delta through the reducer captured on the last
// render, exactly as urql's live handler would.
let acc: unknown;
let lastArgs: { variables?: { cacheID?: string }; pause?: boolean } | undefined;
let lastReducer: ((prev: unknown, frames: unknown[]) => unknown) | undefined;
let connected: boolean;
let lastOpts: { unshared?: boolean } | undefined;

function pushFrame(type: string, entity: unknown, cacheID = lastArgs?.variables?.cacheID) {
  acc = lastReducer!(acc, [{ clusterCachedDataKindsWatch: { type, cacheID, kind: entity } }]);
}

// The Bookmark closing the snapshot: what flips the watch from connecting to live.
function pushBookmark(cacheID = lastArgs?.variables?.cacheID) {
  acc = lastReducer!(acc, [{ clusterCachedDataKindsWatch: { type: 'Bookmark', cacheID, kind: null } }]);
}

beforeEach(() => {
  vi.clearAllMocks();
  acc = undefined;
  lastArgs = undefined;
  lastReducer = undefined;
  connected = true;
  useActiveClusterMock.mockReturnValue({ clusterID: '1', cacheID: 'cache-1', active: true });
  useWatchSubscriptionMock.mockImplementation(
    (args: typeof lastArgs, reducer: typeof lastReducer, opts: { unshared?: boolean } | undefined) => {
      lastArgs = args;
      lastReducer = reducer;
      lastOpts = opts;
      return { data: acc, connected };
    },
  );
});

// Tests ---------------------------------------------------------------

describe('useClusterCachedDataKinds', () => {
  it('accumulates the catalog in the order the cache served it', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments'));
    pushFrame('Added', kind('pods', { apiVersion: 'v1' }));
    pushBookmark();
    rerender();

    expect(resources(result.current.kinds)).toEqual(['deployments', 'pods']);
    expect(result.current.phase).toBe('live');
  });

  it('stays connecting until the Bookmark closes the snapshot', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments'));
    rerender();
    // A partial snapshot is not the catalog: rendering "no kinds" here is the bug.
    expect(result.current.phase).toBe('connecting');

    pushBookmark();
    rerender();
    expect(result.current.phase).toBe('live');
  });

  it('applies a live count change in place, keyed on apiVersion and plural', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments', { count: 1 }));
    pushBookmark();
    pushFrame('Modified', kind('deployments', { count: 7 }));
    rerender();

    expect(result.current.kinds).toHaveLength(1);
    expect(result.current.kinds[0].count).toBe(7);
  });

  it('keeps two groups’ same plural apart', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('widgets', { apiVersion: 'a.example.com/v1' }));
    pushFrame('Added', kind('widgets', { apiVersion: 'b.example.com/v1' }));
    pushBookmark();
    rerender();

    expect(result.current.kinds).toHaveLength(2);
  });

  it('drops a kind the cache stopped serving', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments'));
    pushFrame('Added', kind('pods', { apiVersion: 'v1' }));
    pushBookmark();
    pushFrame('Deleted', kind('pods', { apiVersion: 'v1' }));
    rerender();

    expect(resources(result.current.kinds)).toEqual(['deployments']);
  });

  it('ignores a straggler from a superseded cache', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments'));
    pushBookmark();
    pushFrame('Added', kind('secrets', { apiVersion: 'v1' }), 'cache-0');
    rerender();

    expect(resources(result.current.kinds)).toEqual(['deployments']);
  });

  it('takes its own connection, so a remounted consumer still lists', () => {
    renderHook(() => useClusterCachedDataKinds());
    // The catalog has two consumers and the sidebar's unmounts on collapse; sharing
    // one stream would leave the reopened one folding deltas onto nothing.
    expect(lastOpts?.unshared).toBe(true);
  });

  it('pauses without an active cache, so nothing subscribes', () => {
    useActiveClusterMock.mockReturnValue({ clusterID: undefined, cacheID: undefined, active: false });
    const { result } = renderHook(() => useClusterCachedDataKinds());

    expect(lastArgs?.pause).toBe(true);
    expect(result.current.active).toBe(false);
    expect(result.current.kinds).toEqual([]);
  });

  it('re-subscribes under the new cache when the cluster swaps', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments'));
    pushBookmark();
    rerender();
    expect(result.current.kinds).toHaveLength(1);

    // The previous cache's set must not carry over into the new one's.
    useActiveClusterMock.mockReturnValue({ clusterID: '2', cacheID: 'cache-2', active: true });
    rerender();
    expect(result.current.kinds).toEqual([]);
    expect(result.current.phase).toBe('connecting');
    expect(lastArgs?.variables?.cacheID).toBe('cache-2');
  });

  it('holds the catalog while the transport reconnects', () => {
    const { result, rerender } = renderHook(() => useClusterCachedDataKinds());
    pushFrame('Added', kind('deployments'));
    pushBookmark();
    rerender();

    connected = false;
    rerender();
    expect(result.current.phase).toBe('reconnecting');
    expect(result.current.kinds).toHaveLength(1);
  });
});
