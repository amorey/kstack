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

// The memory popover's skeleton: layout, spacing and color tokens lifted from the
// Figma frame (node 37782:63946). The rail (`MemoryRail`), the viewing-other-
// cluster banner and the instructions block (node 36865:21692, states in
// 37789:57118) are wired for real; saved memories is still a static
// placeholder — it lands component by component on top of this shape.
import { useState } from 'react';
import type { ReactElement, RefObject } from 'react';

import { Popover as PopoverPrimitive } from '@base-ui/react/popover';
import { Pencil, Trash2, X } from 'lucide-react';

import { Button } from '@kubetail/ui/elements/button';
import { PopoverClose } from '@kubetail/ui/elements/popover';
import { Textarea } from '@kubetail/ui/elements/textarea';

import { useActiveKubeContext } from '@/lib/active-kube-context';
import { MemoryRail } from '@/components/widgets/memory-rail';

// Node 36865:21692: empty/filled/editing, same shape SavedMemories' cards are in
// — local state only, since neither reads from nor writes to the sidecar yet.
// The saved text is the parent's, keyed per cluster; this only owns the draft,
// and `MemoryPopoverContent` remounts it on cluster switch (its `key`) so a
// draft never leaks onto another cluster's instructions.
function MemoryInstructions({ savedText, onSave }: { savedText: string | null; onSave: (text: string) => void }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');

  const startEditing = () => {
    setDraft(savedText ?? '');
    setEditing(true);
  };

  const save = () => {
    onSave(draft);
    setEditing(false);
  };

  // Save commits the draft, empty or not; it's disabled only while there's
  // nothing to commit — an empty draft over instructions that were already
  // empty. Clear text only empties the draft, so it's disabled once it
  // already is.
  const saveDisabled = draft === '' && !savedText;
  const clearDisabled = draft === '';

  return (
    <div className="flex flex-col gap-1">
      <p className="text-xs font-medium text-muted-foreground">INSTRUCTIONS</p>
      {editing && (
        <div className="flex w-full flex-col gap-2">
          <Textarea
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            className="h-20 w-full resize-y text-xs"
            autoFocus
          />
          <div className="flex items-center justify-between">
            <button
              type="button"
              onClick={() => setDraft('')}
              disabled={clearDisabled}
              className="text-xs font-medium text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50"
            >
              Clear text
            </button>
            <div className="flex gap-2">
              <Button type="button" size="xs" variant="ghost" onClick={() => setEditing(false)}>
                Cancel
              </Button>
              <Button type="button" size="xs" onClick={save} disabled={saveDisabled}>
                Save
              </Button>
            </div>
          </div>
        </div>
      )}
      {!editing && savedText && (
        <div className="flex items-start gap-3 rounded-md border-l-2 border-primary bg-background px-3 py-2">
          <p className="flex-1 text-xs text-card-foreground">{savedText}</p>
          <button type="button" onClick={startEditing} className="shrink-0 text-xs font-medium text-primary">
            Edit
          </button>
        </div>
      )}
      {!editing && !savedText && (
        <div className="flex items-center justify-between py-2">
          <p className="text-xs text-muted-foreground">No instructions added</p>
          <button type="button" onClick={startEditing} className="text-xs font-medium text-primary">
            Add instructions
          </button>
        </div>
      )}
    </div>
  );
}

function MemoryCard() {
  return (
    <div className="flex w-full shrink-0 flex-col gap-2 rounded-xl border border-border bg-popover px-3.5 py-1.5">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="rounded-md bg-secondary px-2 py-0.5 font-mono text-xs text-secondary-foreground">
            traefik-only
          </span>
          <span className="text-xs text-muted-foreground">by Kstack · 9/20/2026</span>
        </div>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            aria-label="Edit"
            className="flex size-6 items-center justify-center rounded-lg text-muted-foreground"
          >
            <Pencil className="size-4" aria-hidden />
          </button>
          <button
            type="button"
            aria-label="Delete"
            className="flex size-6 items-center justify-center rounded-lg text-muted-foreground"
          >
            <Trash2 className="size-4" aria-hidden />
          </button>
        </div>
      </div>
      <p className="text-xs text-card-foreground">This cluster routes through traefik, not nginx.</p>
    </div>
  );
}

function SavedMemories() {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-1.5">
      <p className="text-xs font-medium text-muted-foreground">SAVED MEMORIES</p>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
        {Array.from({ length: 5 }, (_, i) => (
          <MemoryCard key={i} />
        ))}
      </div>
    </div>
  );
}

// Node 36965:37953: shown only while browsing a cluster other than the window's
// active one — General included, since "another cluster's memory" means a
// cluster, not the cross-cluster bucket.
function ViewingOtherClusterBanner({
  cluster,
  defaultCluster,
  onBackToDefault,
}: {
  cluster: string;
  defaultCluster: string;
  onBackToDefault: () => void;
}) {
  return (
    <div className="flex h-7 w-full shrink-0 items-center gap-2 rounded-lg bg-accent px-3">
      <p className="flex-1 text-xs text-accent-foreground">Viewing {cluster}’s memory.</p>
      <button type="button" onClick={onBackToDefault} className="text-xs font-medium text-primary">
        Back to {defaultCluster}
      </button>
    </div>
  );
}

function MemoryDetail({
  selected,
  defaultCluster,
  onBackToDefault,
  instructions,
  onSaveInstructions,
}: {
  selected: string;
  defaultCluster: string;
  onBackToDefault: () => void;
  instructions: string | null;
  onSaveInstructions: (text: string) => void;
}) {
  const viewingOtherCluster = selected !== defaultCluster && selected !== 'General';

  return (
    <div className="flex h-full min-w-0 flex-1 flex-col gap-[18px] rounded-xl border border-border bg-background p-3">
      {viewingOtherCluster && (
        <ViewingOtherClusterBanner cluster={selected} defaultCluster={defaultCluster} onBackToDefault={onBackToDefault} />
      )}
      <div className="flex min-h-0 flex-1 flex-col gap-6">
        <MemoryInstructions key={selected} savedText={instructions} onSave={onSaveInstructions} />
        <SavedMemories />
      </div>
      <button
        type="button"
        className="h-9 w-full shrink-0 rounded-lg border border-input bg-background text-sm font-medium text-foreground shadow-2xs"
      >
        Add memory
      </button>
    </div>
  );
}

export function MemoryPopoverContent() {
  // The rail's default row is always the window's active cluster — placeholder
  // 'Minikube' until the window has resolved one, matching the other rail rows,
  // which are placeholders too until the real cluster list is wired. `selected`
  // starts unset so it tracks that default live; picking a row fixes it.
  const { context } = useActiveKubeContext();
  const defaultCluster = context || 'Minikube';
  const [selected, setSelected] = useState<string | null>(null);
  // Instructions are per cluster (General is its own bucket), keyed by the rail's
  // row name — local state only, same placeholder scope as the saved memories.
  const [instructionsByCluster, setInstructionsByCluster] = useState<Record<string, string>>({});
  const activeCluster = selected ?? defaultCluster;

  return (
    <div className="flex w-full flex-col gap-3 p-[18px]">
      <div className="flex flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <p className="flex-1 text-base font-medium text-popover-foreground">Memory</p>
          <PopoverClose aria-label="Close" className="text-muted-foreground">
            <X className="size-3.5" aria-hidden />
          </PopoverClose>
        </div>
        <p className="text-xs text-muted-foreground">
          Notes the chats on this cluster read. Kstack saves some as you talk; you can add, edit and delete them.
        </p>
      </div>
      <div className="flex h-96 items-start gap-1">
        <MemoryRail defaultCluster={defaultCluster} selected={activeCluster} onSelect={setSelected} />
        <MemoryDetail
          selected={activeCluster}
          defaultCluster={defaultCluster}
          onBackToDefault={() => setSelected(defaultCluster)}
          instructions={instructionsByCluster[activeCluster] ?? null}
          onSaveInstructions={(text) => setInstructionsByCluster((prev) => ({ ...prev, [activeCluster]: text }))}
        />
      </div>
    </div>
  );
}

// Anchored to the omnibox's own container, not the brain button that triggers
// it: `align="center"` and `w-(--anchor-width)` key off that anchor, so the
// popover tracks the omnibox's center and width as the window's resize — not
// the trigger's — which is narrower.
export function MemoryPopover({
  anchorRef,
  children,
}: {
  anchorRef: RefObject<HTMLElement | null>;
  children: ReactElement;
}) {
  return (
    <PopoverPrimitive.Root>
      <PopoverPrimitive.Trigger render={children} />
      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Positioner anchor={anchorRef} align="center" side="bottom" sideOffset={8}>
          <PopoverPrimitive.Popup className="w-(--anchor-width) rounded-xl border border-border bg-popover text-popover-foreground shadow-lg outline-hidden">
            <MemoryPopoverContent />
          </PopoverPrimitive.Popup>
        </PopoverPrimitive.Positioner>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
