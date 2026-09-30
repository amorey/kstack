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

import { act, fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { Memory } from '@/lib/memories';

// Three seams: the memories watch, the active cluster (the clusters watch behind
// it), and urql's mutation hook.
type ClusterState = { clusterID: string | undefined; uid: string | null };
const { memoriesState, clusterState, saveMock, deleteMock } = vi.hoisted(() => {
  const cluster: { current: ClusterState } = { current: { clusterID: 'c1', uid: 'uid-1' } };
  return {
    memoriesState: { current: { memories: [] as unknown[], phase: 'live' } },
    clusterState: cluster,
    saveMock: vi.fn(),
    deleteMock: vi.fn(),
  };
});
vi.mock('@/lib/memories', () => ({ useMemories: () => memoriesState.current }));
vi.mock('@/lib/active-cluster', () => ({
  useActiveCluster: () => ({
    clusterID: clusterState.current.clusterID,
    cluster: clusterState.current.clusterID ? { status: { server: { uid: clusterState.current.uid } } } : undefined,
  }),
}));
vi.mock('urql', () => ({
  useMutation: (doc: unknown) => [{}, String(JSON.stringify(doc)).includes('memoryDelete') ? deleteMock : saveMock],
}));

const { MemoryDialog } = await import('./memory-dialog');

function memory(over: Partial<Memory> = {}): Memory {
  return {
    id: 'm1',
    clusterID: 'c1',
    name: 'pages',
    body: 'Treat its pods as prod.',
    writtenBy: 'Model',
    updatedAt: '2026-09-20T10:00:00Z',
    ...over,
  };
}

const show = (...memories: Memory[]) => {
  memoriesState.current = { memories, phase: 'live' };
};

const draw = () => render(<MemoryDialog open onOpenChange={() => {}} />);

beforeEach(() => {
  vi.clearAllMocks();
  clusterState.current = { clusterID: 'c1', uid: 'uid-1' };
  show();
  saveMock.mockResolvedValue({ data: { memorySave: { id: 'm1' } } });
  deleteMock.mockResolvedValue({ data: { memoryDelete: true } });
});

describe('MemoryDialog', () => {
  it("groups the cluster's own apart from every cluster's", () => {
    show(memory(), memory({ id: 'm2', clusterID: null, name: 'prefs', writtenBy: 'User' }));
    draw();

    const mine = screen.getByRole('region', { name: 'This cluster' });
    const all = screen.getByRole('region', { name: 'All clusters' });
    expect(within(mine).getByText('pages')).toBeInTheDocument();
    expect(mine).toHaveTextContent('by Kstack');
    expect(within(all).getByText('prefs')).toBeInTheDocument();
    expect(all).toHaveTextContent('by you');
  });

  it('says a group is empty', () => {
    show(memory());
    draw();
    expect(screen.getByRole('region', { name: 'All clusters' })).toHaveTextContent('No memories yet.');
  });

  it('draws a body as its characters, never markup', () => {
    show(memory({ body: '# Heading\n<b>bold</b>' }));
    draw();
    const body = screen.getByText(/# Heading/);
    expect(body).toHaveTextContent('<b>bold</b>');
    expect(body.querySelector('b')).toBeNull();
    expect(document.querySelector('h1')).toBeNull();
  });

  it('says nothing about a rebuild when the cluster has a new UID', () => {
    show(memory());
    clusterState.current = { clusterID: 'c1', uid: 'uid-2' };
    draw();
    expect(screen.queryByText(/rebuilt/)).toBeNull();
  });

  it('waits for a cluster', () => {
    clusterState.current = { clusterID: undefined, uid: null };
    draw();
    expect(screen.getByText('Pick a cluster to see its memories.')).toBeInTheDocument();
  });

  describe('the form', () => {
    const refused = (code: string) => ({ error: { graphQLErrors: [{ extensions: { code } }] } });
    const field = (name: string) => screen.getByLabelText<HTMLInputElement>(name);
    const save = async () => {
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      });
    };

    it('edits a memory', async () => {
      show(memory());
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      expect(field('Name').value).toBe('pages');
      expect(field('Body').value).toBe('Treat its pods as prod.');

      fireEvent.change(field('Name'), { target: { value: 'payments-pages' } });
      fireEvent.change(field('Body'), { target: { value: 'Rewritten.' } });
      await save();

      expect(saveMock).toHaveBeenCalledWith({
        input: {
          id: 'm1',
          clusterID: 'c1',
          name: 'payments-pages',
          body: 'Rewritten.',
        },
      });
      expect(screen.queryByLabelText('Body')).toBeNull();
    });

    // A change that lands while the form is open must not replace the draft: the
    // save writes what the user sees.
    it('saves the draft the form opened with', async () => {
      show(memory());
      const { rerender } = draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));

      show(memory({ body: 'Changed elsewhere.' }));
      rerender(<MemoryDialog open onOpenChange={() => {}} />);
      await save();

      expect(saveMock.mock.calls[0][0].input).toMatchObject({ id: 'm1', body: 'Treat its pods as prod.' });
    });

    // The form sits outside the watched list, so neither a row leaving it nor a
    // reconnect's spinner takes the draft with it.
    it('keeps an edit draft when its memory leaves the list', () => {
      show(memory());
      const { rerender } = draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      fireEvent.change(field('Body'), { target: { value: 'my draft' } });

      show();
      rerender(<MemoryDialog open onOpenChange={() => {}} />);
      expect(field('Body').value).toBe('my draft');
    });

    it('keeps a new draft through a reconnect', () => {
      const { rerender } = draw();
      fireEvent.click(screen.getByRole('button', { name: 'Add a memory' }));
      fireEvent.change(field('Name'), { target: { value: 'oncall' } });

      memoriesState.current = { memories: [], phase: 'connecting' };
      rerender(<MemoryDialog open onOpenChange={() => {}} />);
      expect(field('Name').value).toBe('oncall');
    });

    it('opens one form at a time', () => {
      show(memory());
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      expect(screen.getByRole('button', { name: 'Add a memory' })).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Edit pages' })).toBeDisabled();
    });

    it('locks the form while a save is in flight', async () => {
      show(memory());
      let resolve: (v: unknown) => void = () => {};
      saveMock.mockReturnValue(
        new Promise((r) => {
          resolve = r;
        }),
      );
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));

      expect(field('Body')).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
      await act(async () => {
        resolve({ data: { memorySave: { id: 'm1' } } });
      });
      expect(screen.queryByLabelText('Body')).toBeNull();
    });

    it('moves a memory to every cluster', async () => {
      show(memory());
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      fireEvent.change(field('For'), { target: { value: 'everywhere' } });
      await save();

      expect(saveMock.mock.calls[0][0].input).toMatchObject({ id: 'm1', clusterID: null });
    });

    it('adds a memory to this cluster', async () => {
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Add a memory' }));
      fireEvent.change(field('Name'), { target: { value: 'oncall' } });
      expect(screen.queryByLabelText('Type')).toBeNull();
      expect(screen.queryByLabelText('Description')).toBeNull();
      fireEvent.change(field('Body'), { target: { value: 'https://runbooks.internal/payments' } });
      await save();

      expect(saveMock).toHaveBeenCalledWith({
        input: {
          id: null,
          clusterID: 'c1',
          name: 'oncall',
          body: 'https://runbooks.internal/payments',
        },
      });
    });

    it.each([
      ['KSTACK_RECORD_NOT_FOUND', 'This memory or its cluster is gone.'],
      ['KSTACK_MEMORY_NAME_TAKEN', 'Another memory here has this name.'],
      ['KSTACK_MEMORY_FULL', 'There is no room for this memory here. Shorten or delete one first.'],
      ['KSTACK_MEMORY_SECRET', 'This looks like a credential. Memories never keep one.'],
      [
        'KSTACK_VALIDATION_ERROR',
        'A name is lower-case letters, digits and single hyphens, at most 48 characters, and a memory is at most 500 bytes.',
      ],
    ])('keeps the draft on %s and says why', async (code, message) => {
      show(memory());
      saveMock.mockResolvedValue(refused(code));
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      fireEvent.change(field('Body'), { target: { value: 'my draft' } });
      await save();

      expect(field('Body').value).toBe('my draft');
      expect(screen.getByText(message)).toBeInTheDocument();
    });
  });

  describe('the shape', () => {
    const field = (name: string) => screen.getByLabelText<HTMLInputElement>(name);
    const saveButton = () => screen.getByRole('button', { name: 'Save' });

    it.each([
      ['Name', 'Bad Name', 'Lower-case letters, digits and single hyphens, at most 48 characters.'],
      ['Name', 'a'.repeat(49), 'Lower-case letters, digits and single hyphens, at most 48 characters.'],
      // 251 two-byte characters: under 500 characters, over 500 bytes.
      ['Body', 'é'.repeat(251), 'At most 500 bytes.'],
    ])('draws the rule a %s breaks and sends nothing', (label, value, rule) => {
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Add a memory' }));
      fireEvent.change(field('Name'), { target: { value: 'oncall' } });
      fireEvent.change(field('Body'), { target: { value: 'b' } });
      expect(saveButton()).toBeEnabled();

      fireEvent.change(field(label), { target: { value } });
      expect(screen.getByText(rule)).toBeInTheDocument();
      expect(saveButton()).toBeDisabled();
      fireEvent.click(saveButton());
      expect(saveMock).not.toHaveBeenCalled();
    });

    it('holds Save until the name and the body are there', () => {
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Add a memory' }));
      expect(saveButton()).toBeDisabled();
      expect(screen.queryByText(/at most/)).toBeNull();
    });
  });

  describe('adopting', () => {
    const line = 'Saving makes this your memory: Kstack will follow it.';

    it("says saving one of Kstack's memories makes it yours", () => {
      show(memory({ writtenBy: 'Model' }));
      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      expect(screen.getByText(line)).toBeInTheDocument();
    });

    it('says nothing for a memory already yours, or a new one', () => {
      show(memory({ writtenBy: 'User' }));
      const { unmount } = draw();
      fireEvent.click(screen.getByRole('button', { name: 'Edit pages' }));
      expect(screen.queryByText(line)).toBeNull();
      unmount();

      draw();
      fireEvent.click(screen.getByRole('button', { name: 'Add a memory' }));
      expect(screen.queryByText(line)).toBeNull();
    });
  });

  it('deletes only once confirmed', async () => {
    show(memory());
    draw();
    fireEvent.click(screen.getByRole('button', { name: 'Delete pages' }));
    expect(deleteMock).not.toHaveBeenCalled();

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete memory' }));
    });
    expect(deleteMock).toHaveBeenCalledWith({ id: 'm1' });
  });

  it('keeps the confirm and says so when a delete fails', async () => {
    deleteMock.mockResolvedValue({ error: new Error('socket closed') });
    show(memory());
    draw();
    fireEvent.click(screen.getByRole('button', { name: 'Delete pages' }));

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Delete memory' }));
    });
    expect(screen.getByText('The memory could not be deleted.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete memory' })).toBeInTheDocument();
  });
});
