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

const { saveMock, deleteMock } = vi.hoisted(() => ({ saveMock: vi.fn(), deleteMock: vi.fn() }));
vi.mock('urql', () => ({
  useMutation: (doc: unknown) => [{}, String(JSON.stringify(doc)).includes('memoryDelete') ? deleteMock : saveMock],
}));

const { bodyFits, nameFits, useMemoryWrites } = await import('./writes');

const refused = (code: string) => ({ error: { graphQLErrors: [{ extensions: { code } }] } });
const writes = () => renderHook(() => useMemoryWrites()).result.current;
const input = { id: 'm1', clusterID: 'c1', name: 'pages', body: 'Treat its pods as prod.' };

beforeEach(() => {
  vi.clearAllMocks();
  saveMock.mockResolvedValue({ data: { memorySave: { id: 'm1' } } });
  deleteMock.mockResolvedValue({ data: { memoryDelete: true } });
});

describe('useMemoryWrites', () => {
  it('sends a save as given and answers null', async () => {
    expect(await writes().save(input)).toBeNull();
    expect(saveMock).toHaveBeenCalledWith({ input });
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
    ['KSTACK_SOMETHING_ELSE', 'The memory could not be saved.'],
  ])('says why a save was refused with %s', async (code, message) => {
    saveMock.mockResolvedValue(refused(code));
    expect(await writes().save(input)).toBe(message);
  });

  it('deletes every id and answers the ones still there', async () => {
    expect(await writes().remove(['m1', 'm2'])).toEqual([]);
    expect(deleteMock.mock.calls).toEqual([[{ id: 'm1' }], [{ id: 'm2' }]]);

    deleteMock.mockResolvedValueOnce({ data: { memoryDelete: true } });
    deleteMock.mockResolvedValueOnce({ error: { graphQLErrors: [], message: 'socket closed' } });
    expect(await writes().remove(['m1', 'm2'])).toEqual(['m2']);
  });

  it('counts a memory already gone as deleted', async () => {
    deleteMock.mockResolvedValue(refused('KSTACK_RECORD_NOT_FOUND'));
    expect(await writes().remove(['m1'])).toEqual([]);
  });
});

describe('the shape rules', () => {
  it('takes lower-case words joined by single hyphens, up to 48 characters', () => {
    expect(nameFits('payments-pages')).toBe(true);
    expect(nameFits('Bad Name')).toBe(false);
    expect(nameFits('double--hyphen')).toBe(false);
    expect(nameFits('a'.repeat(49))).toBe(false);
  });

  it('measures a body in bytes', () => {
    expect(bodyFits('é'.repeat(250))).toBe(true);
    expect(bodyFits('é'.repeat(251))).toBe(false);
  });
});
