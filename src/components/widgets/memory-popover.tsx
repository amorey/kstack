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
// 37789:57118) are wired for real. Saved memories (node 37821:58028, editing
// flow in 37826:6588) is wired the same way — local state only, since neither
// reads from nor writes to the sidecar yet.
import { Fragment, useState } from 'react';
import type { ReactElement, RefObject } from 'react';

import { Checkbox as CheckboxPrimitive } from '@base-ui/react/checkbox';
import { Popover as PopoverPrimitive } from '@base-ui/react/popover';
import { Check, Minus, MoreHorizontal, X } from 'lucide-react';

import { Button } from '@kubetail/ui/elements/button';
import { Checkbox } from '@kubetail/ui/elements/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kubetail/ui/elements/dropdown-menu';
import { PopoverClose } from '@kubetail/ui/elements/popover';
import { Textarea } from '@kubetail/ui/elements/textarea';

import { Dialog } from '@/components/widgets/dialog';
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
    <div className="flex flex-col gap-1.5">
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
            <div className="flex gap-1.5">
              <Button
                type="button"
                size="xs"
                variant="ghost"
                className="h-7 px-3 py-2"
                onClick={() => setEditing(false)}
              >
                Cancel
              </Button>
              <Button type="button" size="xs" className="h-7 px-3 py-2" onClick={save} disabled={saveDisabled}>
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

type MemoryEntry = { id: string; name: string; body: string };

// Node 37821:58028's five placeholder rows — same cluster-agnostic content as
// the old static card, just given distinct ids so a row survives edit/delete.
const PLACEHOLDER_MEMORIES: MemoryEntry[] = Array.from({ length: 5 }, (_, i) => ({
  id: `placeholder-${i}`,
  name: 'traefik-only',
  body: 'This cluster routes through traefik, not nginx.',
}));

// Node 37826:7321 (editing): a row becomes its own textarea in place, same
// Cancel/Save shape as `MemoryInstructions`. Provenance (name + date) stays
// read-only under the field while editing.
function MemoryCardRow({
  memory,
  selectMode,
  selected,
  editing,
  onToggleSelected,
  onStartEditing,
  onStopEditing,
  onSave,
  onRequestDelete,
}: {
  memory: MemoryEntry;
  selectMode: boolean;
  selected: boolean;
  editing: boolean;
  onToggleSelected: () => void;
  onStartEditing: () => void;
  onStopEditing: () => void;
  onSave: (body: string) => void;
  onRequestDelete: () => void;
}) {
  const [draft, setDraft] = useState(memory.body);
  const [menuOpen, setMenuOpen] = useState(false);

  const startEditing = () => {
    setDraft(memory.body);
    onStartEditing();
  };

  const save = () => {
    const next = draft.trim();
    if (next) onSave(next);
    onStopEditing();
  };

  if (editing) {
    return (
      <div className="flex w-full shrink-0 flex-col gap-1.5 pt-0.5 pb-1">
        <Textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          className="h-14 w-full resize-y text-xs"
          autoFocus
        />
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium">{memory.name}</span>
          <span>by Kstack · 9/20/2026</span>
        </div>
        <div className="flex justify-end gap-1.5">
          <Button type="button" size="xs" variant="ghost" className="h-7 px-3 py-2" onClick={onStopEditing}>
            Cancel
          </Button>
          <Button type="button" size="xs" className="h-7 px-3 py-2" onClick={save} disabled={draft.trim() === ''}>
            Save
          </Button>
        </div>
      </div>
    );
  }

  // Select mode reveals every row's checkbox and menu, but only a selected
  // (or this row's own open-menu) row gets a fill — an unselected row in
  // select mode is still plain, tinting only on a real :hover.
  const revealed = selectMode || menuOpen ? 'flex' : 'hidden group-hover/card:flex';
  // Select mode indents every row the same amount, selected or not, so the
  // checkbox column lines up — an unselected row stays unfilled until hover.
  let tint = selectMode ? 'rounded-xl px-1.5 hover:bg-secondary' : 'hover:rounded-xl hover:bg-secondary hover:px-1.5';
  // Hover is a CSS :hover, so the moment the cursor leaves the row for its
  // own open menu it would otherwise revert to plain — hiding the trigger
  // mid-click (dropping the menu's anchor, so it snaps to the top-left
  // corner) and collapsing the row's fill under the open menu. `menuOpen`
  // pins the fill for as long as the row's own menu is open.
  if (selected) tint = 'rounded-xl bg-card px-1.5';
  else if (menuOpen) tint = 'rounded-xl bg-secondary px-1.5';

  return (
    <div className={`group/card flex w-full shrink-0 items-start gap-2 py-3 ${tint}`}>
      <Checkbox checked={selected} onCheckedChange={onToggleSelected} className={`mt-0.5 ${revealed}`} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="text-sm text-card-foreground">{memory.body}</p>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium">{memory.name}</span>
          <span>by Kstack · 9/20/2026</span>
        </div>
      </div>
      <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
        <DropdownMenuTrigger
          aria-label={`Actions for ${memory.name}`}
          className={`size-6 shrink-0 items-center justify-center rounded-lg text-muted-foreground outline-none ${revealed}`}
        >
          <MoreHorizontal className="size-4" aria-hidden />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" sideOffset={4}>
          <DropdownMenuItem onClick={startEditing}>Edit</DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onClick={onRequestDelete}>
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

// `@kubetail/ui`'s `Checkbox` always draws a check mark, indeterminate included
// (base-ui only toggles the `data-indeterminate` attribute, not the icon), so
// the select-all control needs its own indicator to show the dash.
function SelectAllCheckbox({
  checked,
  indeterminate,
  onCheckedChange,
}: {
  checked: boolean;
  indeterminate: boolean;
  onCheckedChange: () => void;
}) {
  return (
    <CheckboxPrimitive.Root
      checked={checked}
      indeterminate={indeterminate}
      onCheckedChange={onCheckedChange}
      className="relative mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-[4px] border border-input bg-background outline-none data-checked:border-primary data-checked:bg-primary data-checked:text-primary-foreground data-indeterminate:border-primary data-indeterminate:bg-primary data-indeterminate:text-primary-foreground"
    >
      <CheckboxPrimitive.Indicator className="grid place-content-center text-current [&>svg]:size-3.5">
        {indeterminate ? <Minus /> : <Check />}
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

// Node 36902:2631: replaces the SAVED MEMORIES heading while any row is
// checked. The leading checkbox selects or clears every row at once.
function BulkActionBar({
  selectedCount,
  total,
  allSelected,
  onToggleSelectAll,
  onCancel,
  onDelete,
}: {
  selectedCount: number;
  total: number;
  allSelected: boolean;
  onToggleSelectAll: () => void;
  onCancel: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="flex w-full shrink-0 items-center justify-between px-1.5">
      <div className="flex items-start gap-2.5">
        <SelectAllCheckbox
          checked={allSelected}
          indeterminate={!allSelected && selectedCount > 0}
          onCheckedChange={onToggleSelectAll}
        />
        <div className="flex flex-col text-xs leading-4">
          <span className="font-medium text-foreground">{selectedCount} selected</span>
          <span className="text-muted-foreground">{total} memories in total</span>
        </div>
      </div>
      <div className="flex items-center gap-1">
        <Button type="button" size="xs" variant="ghost" className="h-7" onClick={onCancel}>
          Cancel
        </Button>
        {/* Node 36872:23599: a solid fill, not the library's soft `destructive`
            chip (light bg, red text) — this is one of two Figma destructive
            treatments, the other being the soft one on each card's menu. */}
        <Button
          type="button"
          size="xs"
          className="h-7 bg-destructive text-destructive-foreground hover:bg-destructive/90"
          onClick={onDelete}
        >
          Delete ({selectedCount})
        </Button>
      </div>
    </div>
  );
}

function SavedMemories({
  cluster,
  memories,
  onSave,
  onDelete,
}: {
  cluster: string;
  memories: MemoryEntry[];
  onSave: (id: string, body: string) => void;
  onDelete: (ids: string[]) => void;
}) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  // The ids a confirm dialog is pending for — one row's from its own menu, or
  // the bulk bar's whole selection.
  const [confirmIds, setConfirmIds] = useState<string[] | null>(null);
  // At most one row edits at a time — opening another row's edit discards
  // whatever the previous one had in progress.
  const [editingId, setEditingId] = useState<string | null>(null);
  const selectMode = selectedIds.size > 0;

  const toggleSelected = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    setSelectedIds((prev) => (prev.size === memories.length ? new Set() : new Set(memories.map((m) => m.id))));
  };

  const confirmDelete = () => {
    if (confirmIds) {
      onDelete(confirmIds);
      setSelectedIds((prev) => {
        const next = new Set(prev);
        confirmIds.forEach((id) => next.delete(id));
        return next;
      });
    }
    setConfirmIds(null);
  };

  const confirmCount = confirmIds?.length ?? 0;
  const confirmNoun = confirmCount === 1 ? 'memory' : 'memories';

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-0.5">
      {selectMode ? (
        <BulkActionBar
          selectedCount={selectedIds.size}
          total={memories.length}
          allSelected={selectedIds.size === memories.length}
          onToggleSelectAll={toggleSelectAll}
          onCancel={() => setSelectedIds(new Set())}
          onDelete={() => setConfirmIds(Array.from(selectedIds))}
        />
      ) : (
        <p className="text-xs font-medium text-muted-foreground">SAVED MEMORIES</p>
      )}
      {/* Node 37826:7567: pl-1/pr-3 on the list itself, not per row — room for
          an edited row's focus ring to render in full without clipping. */}
      <div className={`flex min-h-0 flex-1 flex-col overflow-y-auto pr-3 pl-1 ${selectMode ? 'gap-0.5' : ''}`}>
        {memories.length === 0 && <p className="py-2 text-xs text-muted-foreground">No saved memories yet.</p>}
        {memories.map((memory, i) => (
          // Node 37796:67770: the divider insets 12px off each edge, a 0.5px
          // muted stroke — a plain `divide-y` can't inset, so it's its own
          // element between rows instead.
          <Fragment key={memory.id}>
            {!selectMode && i > 0 && <div className="mx-3 border-t-[0.5px] border-muted" />}
            <MemoryCardRow
              memory={memory}
              selectMode={selectMode}
              selected={selectedIds.has(memory.id)}
              editing={editingId === memory.id}
              onToggleSelected={() => toggleSelected(memory.id)}
              onStartEditing={() => setEditingId(memory.id)}
              onStopEditing={() => setEditingId(null)}
              onSave={(body) => onSave(memory.id, body)}
              onRequestDelete={() => setConfirmIds([memory.id])}
            />
          </Fragment>
        ))}
      </div>
      <Dialog
        open={confirmIds !== null}
        onOpenChange={(open) => !open && setConfirmIds(null)}
        title={`Delete ${confirmCount} ${confirmNoun}?`}
        description={`The ${confirmCount} selected ${confirmNoun} will be permanently deleted from ${cluster}. This cannot be undone.`}
      >
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => setConfirmIds(null)}>
            Cancel
          </Button>
          <Button
            type="button"
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={confirmDelete}
          >
            Delete {confirmCount} {confirmNoun}
          </Button>
        </div>
      </Dialog>
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
  memories,
  onSaveMemory,
  onDeleteMemories,
}: {
  selected: string;
  defaultCluster: string;
  onBackToDefault: () => void;
  instructions: string | null;
  onSaveInstructions: (text: string) => void;
  memories: MemoryEntry[];
  onSaveMemory: (id: string, body: string) => void;
  onDeleteMemories: (ids: string[]) => void;
}) {
  const viewingOtherCluster = selected !== defaultCluster && selected !== 'General';

  return (
    <div className="flex h-full min-w-0 flex-1 flex-col gap-[18px] rounded-xl border border-border bg-background p-3">
      {viewingOtherCluster && (
        <ViewingOtherClusterBanner
          cluster={selected}
          defaultCluster={defaultCluster}
          onBackToDefault={onBackToDefault}
        />
      )}
      <div className="flex min-h-0 flex-1 flex-col gap-6">
        <MemoryInstructions key={`instructions-${selected}`} savedText={instructions} onSave={onSaveInstructions} />
        <SavedMemories
          key={`memories-${selected}`}
          cluster={selected}
          memories={memories}
          onSave={onSaveMemory}
          onDelete={onDeleteMemories}
        />
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
  // Saved memories are per cluster too; a cluster not yet touched falls back to
  // the placeholder five so every row still looks like real content.
  const [memoriesByCluster, setMemoriesByCluster] = useState<Record<string, MemoryEntry[]>>({});
  const activeCluster = selected ?? defaultCluster;
  const activeMemories = memoriesByCluster[activeCluster] ?? PLACEHOLDER_MEMORIES;

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
          memories={activeMemories}
          onSaveMemory={(id, body) =>
            setMemoriesByCluster((prev) => ({
              ...prev,
              [activeCluster]: activeMemories.map((m) => (m.id === id ? { ...m, body } : m)),
            }))
          }
          onDeleteMemories={(ids) =>
            setMemoriesByCluster((prev) => ({
              ...prev,
              [activeCluster]: activeMemories.filter((m) => !ids.includes(m.id)),
            }))
          }
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
