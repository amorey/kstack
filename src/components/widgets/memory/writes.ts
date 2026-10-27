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

// The user's memory writes to the sidecar, and the shape rules checked as they
// type. Every save from here marks the memory as the user's.
import { useMutation } from 'urql';

import { graphql } from '@/gql';

const MemorySaveMutation = graphql(`
  mutation MemorySave($input: MemorySaveInput!) {
    memorySave(input: $input) {
      id
    }
  }
`);

const MemoryDeleteMutation = graphql(`
  mutation MemoryDelete($id: MemoryID!) {
    memoryDelete(id: $id)
  }
`);

// The sidecar's shape rules (tools.CheckMemory), checked as the user types: its
// refusal names no field, so the form says which rule a field breaks.
const NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const NAME_MAX = 48;
const BODY_MAX_BYTES = 500;
export const NAME_RULE = `Lower-case letters, digits and single hyphens, at most ${NAME_MAX} characters.`;
export const BODY_RULE = `At most ${BODY_MAX_BYTES} bytes.`;

const utf8 = new TextEncoder();
export const nameFits = (name: string) => name.length <= NAME_MAX && NAME_RE.test(name);
export const bodyFits = (body: string) => utf8.encode(body).length <= BODY_MAX_BYTES;

// Why a save was refused, by the code the sidecar sent.
const REFUSALS: Record<string, string> = {
  KSTACK_RECORD_NOT_FOUND: 'This memory or its cluster is gone.',
  KSTACK_MEMORY_NAME_TAKEN: 'Another memory here has this name.',
  KSTACK_MEMORY_FULL: 'There is no room for this memory here. Shorten or delete one first.',
  KSTACK_MEMORY_SECRET: 'This looks like a credential. Memories never keep one.',
  KSTACK_VALIDATION_ERROR: `A name is lower-case letters, digits and single hyphens, at most ${NAME_MAX} characters, and a memory is at most ${BODY_MAX_BYTES} bytes.`,
};

// `clusterID` null saves the memory for every cluster.
export type MemorySaveInput = { id: string | null; clusterID: string | null; name: string; body: string };

export function useMemoryWrites(): {
  // Resolves to why the sidecar refused, or null once saved.
  save: (input: MemorySaveInput) => Promise<string | null>;
  // Resolves to the ids still there. One already gone counts as deleted, so
  // retrying a delete that partly failed can succeed.
  remove: (ids: string[]) => Promise<string[]>;
} {
  const [, memorySave] = useMutation(MemorySaveMutation);
  const [, memoryDelete] = useMutation(MemoryDeleteMutation);

  const save = async (input: MemorySaveInput) => {
    const result = await memorySave({ input });
    if (!result.error) return null;
    const code = result.error.graphQLErrors[0]?.extensions?.code;
    return REFUSALS[String(code)] ?? 'The memory could not be saved.';
  };

  const remove = async (ids: string[]) => {
    const results = await Promise.all(ids.map((id) => memoryDelete({ id })));
    return ids.filter((_, i) => {
      const { error } = results[i];
      return error && error.graphQLErrors[0]?.extensions?.code !== 'KSTACK_RECORD_NOT_FOUND';
    });
  };

  return { save, remove };
}
