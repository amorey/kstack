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

// The memory popover's left rail (Figma's `.memoryrail`, node 36946:37550): a
// Cluster group that collapses to the scoped memories, and a General row beside
// it for memories that aren't scoped to any cluster. Selecting a row is what
// decides the detail pane beside it — not wired yet, see `memory-popover.tsx`.
import { useState } from 'react';

import { ChevronDown } from 'lucide-react';

import { cn } from '@kubetail/ui/lib/utils';

// Selected takes the sidebar's accent permanently, unselected takes it only on
// hover. The focus ring is the app's own (`ring-ring`), not Figma's one-off
// neutral gray — this popover shouldn't carry a second ring color the rest of
// the app doesn't use.
const ROW_BASE = 'rounded-lg p-2 text-left text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring';

// A scoped cluster row: 28px, medium weight only once selected.
const clusterRowClass = (selected: boolean) =>
  cn(
    ROW_BASE,
    'flex h-7 items-center',
    selected
      ? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
      : 'font-normal text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
  );

// General: a group label's height (32px) and weight (always medium) — it reads
// as a header until it's selected, then it reads as a row like the others.
const generalRowClass = (selected: boolean) =>
  cn(
    ROW_BASE,
    'flex h-8 w-full items-center font-medium',
    selected
      ? 'bg-sidebar-accent text-sidebar-accent-foreground'
      : 'text-sidebar-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
  );

const RAIL_CLUSTERS = ['Minikube', '2nd Cluster', '3rd Cluster', '4th Cluster'];

export function MemoryRail() {
  const [clusterOpen, setClusterOpen] = useState(true);
  // Which row reads as selected. Placeholder state: nothing downstream reads it
  // yet — the detail pane starts following it once that component is wired.
  const [selected, setSelected] = useState('Minikube');

  return (
    <div className="flex h-full w-44 shrink-0 flex-col gap-2">
      <div>
        {/* Clickable to expand/collapse, but — unlike a row below it — never takes
            the accent hover: it toggles the group, it doesn't select anything. */}
        <button
          type="button"
          onClick={() => setClusterOpen((open) => !open)}
          aria-expanded={clusterOpen}
          className="flex w-full items-center gap-2 rounded-lg px-2 pt-2 pb-1 text-left text-sm font-medium text-sidebar-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="flex-1">Cluster</span>
          <ChevronDown
            className={cn('size-4 shrink-0 transition-transform', !clusterOpen && '-rotate-90')}
            aria-hidden
          />
        </button>
        {clusterOpen && (
          <div className="flex flex-col gap-1 px-1.5 py-0.5">
            {RAIL_CLUSTERS.map((name) => (
              <button
                key={name}
                type="button"
                aria-pressed={selected === name}
                onClick={() => setSelected(name)}
                className={clusterRowClass(selected === name)}
              >
                {name}
              </button>
            ))}
          </div>
        )}
      </div>
      <button
        type="button"
        aria-pressed={selected === 'General'}
        onClick={() => setSelected('General')}
        className={generalRowClass(selected === 'General')}
      >
        General
      </button>
    </div>
  );
}
