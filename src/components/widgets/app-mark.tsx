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

// The app's mark alone: a lettered square. A letter rather than an asset — text
// needs no bundle entry. Decorative, so whatever draws it says the name itself.
// index.html carries a static copy for the frames before React mounts; that copy
// cannot import this, so it restates the 48px shape ReadyGate draws.
export function AppMark({ size }: { size: 24 | 48 }) {
  return (
    <span
      aria-hidden
      data-testid="app-mark"
      className={
        size === 48
          ? 'flex size-12 shrink-0 items-center justify-center rounded-xl bg-primary text-2xl font-bold text-primary-foreground'
          : 'flex size-6 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground'
      }
    >
      K
    </span>
  );
}
