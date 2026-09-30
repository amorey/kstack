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

// What the window's cluster remembers: its own memories and every cluster's, each
// listed, read, edited and deleted here. A memory may have been written by the
// model from cluster data, so every field is drawn as text, never as markdown.
import { useId, useState } from 'react';
import { useMutation } from 'urql';

import { Button } from '@kubetail/ui/elements/button';
import { Field, FieldLabel } from '@kubetail/ui/elements/field';
import { Input } from '@kubetail/ui/elements/input';
import { Spinner } from '@kubetail/ui/elements/spinner';
import { Textarea } from '@kubetail/ui/elements/textarea';

import { Dialog } from '@/components/widgets/dialog';
import { graphql } from '@/gql';
import { useActiveCluster } from '@/lib/active-cluster';
import type { AppDialogProps } from '@/lib/dialog';
import { useMemories } from '@/lib/memories';
import type { Memory } from '@/lib/memories';

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

// The sidecar's shape rules (tools.CheckMemory), checked here as the user types:
// its refusal names no field, so the form says which rule a field breaks.
const NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const NAME_MAX = 48;
const BODY_MAX_BYTES = 500;
const NAME_RULE = `Lower-case letters, digits and single hyphens, at most ${NAME_MAX} characters.`;
const BODY_RULE = `At most ${BODY_MAX_BYTES} bytes.`;

const utf8 = new TextEncoder();
const nameFits = (name: string) => name.length <= NAME_MAX && NAME_RE.test(name);
const bodyFits = (body: string) => utf8.encode(body).length <= BODY_MAX_BYTES;

// Why a save was refused, in the dialog's words, by the code the sidecar sent.
const REFUSALS: Record<string, string> = {
  KSTACK_RECORD_NOT_FOUND: 'This memory or its cluster is gone.',
  KSTACK_MEMORY_NAME_TAKEN: 'Another memory here has this name.',
  KSTACK_MEMORY_FULL: 'There is no room for this memory here. Shorten or delete one first.',
  KSTACK_MEMORY_SECRET: 'This looks like a credential. Memories never keep one.',
  KSTACK_VALIDATION_ERROR: `A name is lower-case letters, digits and single hyphens, at most ${NAME_MAX} characters, and a memory is at most ${BODY_MAX_BYTES} bytes.`,
};

type Draft = { name: string; scope: 'cluster' | 'everywhere'; body: string };

// The library has no select, so the select borrows its input's look.
const SELECT = 'h-9 w-full rounded-md border bg-transparent px-3 text-sm';

// What the open form edits, fixed when it opens: the memory as it was read, none
// for a new one, and the cluster the window was on. Later watch frames and a
// cluster switch leave it alone, so the draft saves where it was read.
type Editing = { memory?: Memory; clusterID: string };

// One memory as the user writes it. A refused save keeps the draft, and the form
// is locked while a save is in flight, so nothing typed is lost.
function MemoryForm({ editing: { memory, clusterID }, onDone }: { editing: Editing; onDone: () => void }) {
  const [, memorySave] = useMutation(MemorySaveMutation);
  const [draft, setDraft] = useState<Draft>({
    name: memory?.name ?? '',
    scope: memory && memory.clusterID === null ? 'everywhere' : 'cluster',
    body: memory?.body ?? '',
  });
  const [saving, setSaving] = useState(false);
  const [refusal, setRefusal] = useState('');
  const set = (patch: Partial<Draft>) => setDraft((prev) => ({ ...prev, ...patch }));
  const id = useId();
  const nameBroken = draft.name !== '' && !nameFits(draft.name);
  const bodyBroken = !bodyFits(draft.body);
  const ready = draft.name !== '' && draft.body !== '' && !nameBroken && !bodyBroken;

  const save = async () => {
    setSaving(true);
    setRefusal('');
    const result = await memorySave({
      input: {
        id: memory?.id ?? null,
        clusterID: draft.scope === 'cluster' ? (memory?.clusterID ?? clusterID) : null,
        name: draft.name,
        body: draft.body,
      },
    });
    if (result.error) {
      const code = result.error.graphQLErrors[0]?.extensions?.code;
      setRefusal(REFUSALS[String(code)] ?? 'The memory could not be saved.');
      setSaving(false);
      return;
    }
    onDone();
  };

  return (
    <form
      className="mb-4 rounded-md border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) save();
      }}
    >
      <h3 className="mb-2 text-sm font-medium">{memory ? `Edit ${memory.name}` : 'New memory'}</h3>
      <fieldset disabled={saving} className="flex flex-col gap-2">
        <Field>
          <FieldLabel htmlFor={`${id}-name`}>Name</FieldLabel>
          <Input
            id={`${id}-name`}
            className="font-mono"
            value={draft.name}
            onChange={(e) => set({ name: e.target.value })}
          />
          {nameBroken && <p className="text-xs text-destructive">{NAME_RULE}</p>}
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-scope`}>For</FieldLabel>
          <select
            id={`${id}-scope`}
            className={SELECT}
            value={draft.scope}
            onChange={(e) => set({ scope: e.target.value === 'everywhere' ? 'everywhere' : 'cluster' })}
          >
            <option value="cluster">This cluster</option>
            <option value="everywhere">All clusters</option>
          </select>
        </Field>
        <Field>
          <FieldLabel htmlFor={`${id}-body`}>Body</FieldLabel>
          <Textarea id={`${id}-body`} value={draft.body} onChange={(e) => set({ body: e.target.value })} />
          {bodyBroken && <p className="text-xs text-destructive">{BODY_RULE}</p>}
        </Field>
        {refusal !== '' && <p className="text-xs text-destructive">{refusal}</p>}
        <div className="flex gap-2">
          <Button type="submit" size="xs" disabled={!ready}>
            Save
          </Button>
          <Button type="button" size="xs" variant="outline" onClick={onDone}>
            Cancel
          </Button>
        </div>
        {memory?.writtenBy === 'Model' && (
          <p className="text-xs text-muted-foreground">Saving makes this your memory: Kstack will follow it.</p>
        )}
      </fieldset>
    </form>
  );
}

type RowProps = {
  memory: Memory;
  /** Opens the form on this memory; undefined while a form is open. */
  onEdit?: (memory: Memory) => void;
};

function MemoryRow({ memory, onEdit }: RowProps) {
  const [, memoryDelete] = useMutation(MemoryDeleteMutation);
  const [confirming, setConfirming] = useState(false);
  const [failed, setFailed] = useState(false);
  // A failed delete keeps the confirm row, so the user sees why nothing went.
  const remove = async () => {
    setFailed(false);
    const result = await memoryDelete({ id: memory.id });
    if (result.error) {
      setFailed(true);
      return;
    }
    setConfirming(false);
  };
  return (
    <li className="border-b py-2 last:border-b-0">
      <details>
        <summary className="cursor-pointer select-none">
          <span className="font-mono text-sm break-all">{memory.name}</span>
          <span className="ml-2 text-xs text-muted-foreground">
            {new Date(memory.updatedAt).toLocaleDateString()} · {memory.writtenBy === 'User' ? 'by you' : 'by Kstack'}
          </span>
        </summary>
        <p className="mt-1 border-l-2 border-muted pl-3 text-sm break-words whitespace-pre-wrap">{memory.body}</p>
        <div className="mt-2 flex gap-2">
          {confirming ? (
            <>
              <Button type="button" size="xs" variant="destructive" onClick={remove}>
                Delete memory
              </Button>
              <Button
                type="button"
                size="xs"
                variant="outline"
                onClick={() => {
                  setConfirming(false);
                  setFailed(false);
                }}
              >
                Keep it
              </Button>
              {failed && <p className="self-center text-xs text-destructive">The memory could not be deleted.</p>}
            </>
          ) : (
            <>
              <Button
                type="button"
                size="xs"
                variant="outline"
                aria-label={`Edit ${memory.name}`}
                disabled={!onEdit}
                onClick={() => onEdit?.(memory)}
              >
                Edit
              </Button>
              <Button
                type="button"
                size="xs"
                variant="outline"
                aria-label={`Delete ${memory.name}`}
                onClick={() => setConfirming(true)}
              >
                Delete
              </Button>
            </>
          )}
        </div>
      </details>
    </li>
  );
}

function Group({ title, memories, onEdit }: { title: string; memories: Memory[] } & Omit<RowProps, 'memory'>) {
  return (
    <section aria-label={title} className="mb-4">
      <h3 className="mb-1 text-sm font-medium">{title}</h3>
      {memories.length === 0 ? (
        <p className="text-xs text-muted-foreground">No memories yet.</p>
      ) : (
        <ul>
          {memories.map((m) => (
            <MemoryRow key={m.id} memory={m} onEdit={onEdit} />
          ))}
        </ul>
      )}
    </section>
  );
}

export function MemoryDialog({ open, onOpenChange }: AppDialogProps) {
  const { clusterID } = useActiveCluster();
  const { memories, phase } = useMemories(clusterID);
  // One form at a time, held here rather than in a row: a row leaves the list when
  // its memory is deleted or moves, and the list gives way to a spinner on a
  // reconnect, and neither may take a draft with it.
  const [editing, setEditing] = useState<Editing | null>(null);
  const onEdit = editing || !clusterID ? undefined : (memory: Memory) => setEditing({ memory, clusterID });

  let content;
  if (!clusterID) {
    content = <p className="text-sm text-muted-foreground">Pick a cluster to see its memories.</p>;
  } else if (phase === 'connecting') {
    content = <Spinner />;
  } else {
    content = (
      <>
        <Group title="This cluster" memories={memories.filter((m) => m.clusterID !== null)} onEdit={onEdit} />
        <Group title="All clusters" memories={memories.filter((m) => m.clusterID === null)} onEdit={onEdit} />
        <Button
          type="button"
          size="xs"
          variant="outline"
          disabled={editing !== null}
          onClick={() => setEditing({ clusterID })}
        >
          Add a memory
        </Button>
      </>
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Memory"
      description="Notes the chats on this cluster read. Kstack saves some as you talk; you can add, edit and delete them."
      className="sm:max-w-2xl"
    >
      {editing && <MemoryForm editing={editing} onDone={() => setEditing(null)} />}
      {content}
    </Dialog>
  );
}
