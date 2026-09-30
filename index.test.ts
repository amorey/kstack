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

import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

// index.html's inline style hand-mirrors the mark's tokens from src/index.css, since
// the sheet is not on the page for the first frames. A token change must fail here
// rather than desynchronise the first paint.

const html = readFileSync('index.html', 'utf8');
const css = readFileSync('src/index.css', 'utf8');

// The value of `name` in the first `{ ... }` block that follows `selector`.
function token(source: string, selector: string, name: string): string {
  const block = source.slice(source.indexOf(selector));
  const body = block.slice(block.indexOf('{'), block.indexOf('}'));
  const at = body.indexOf(`${name}:`);
  if (at < 0) throw new Error(`${name} not found under ${selector}`);
  return body.slice(at + name.length + 1, body.indexOf(';', at)).trim();
}

describe('index.html boot mark', () => {
  it.each([
    ['light', ':root', '.boot-mark {'],
    ['dark', '.dark {', '.dark .boot-mark'],
  ])('mirrors the %s tokens', (_scheme, cssSelector, htmlSelector) => {
    expect(token(html, htmlSelector, 'background')).toBe(token(css, cssSelector, '--primary'));
    expect(token(html, htmlSelector, 'color')).toBe(token(css, cssSelector, '--primary-foreground'));
  });

  it('places the static mark inside #root, where the mount replaces it', () => {
    expect(html).toMatch(/<div id="root"><div class="boot-mark" aria-hidden="true">K<\/div><\/div>/);
  });
});
