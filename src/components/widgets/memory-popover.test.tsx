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

import { Popover as PopoverPrimitive } from '@base-ui/react/popover';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/active-kube-context', () => ({
  useActiveKubeContext: () => ({ context: 'Minikube' }),
}));

const { MemoryPopoverContent } = await import('./memory-popover');

// `PopoverClose` in the header needs a `Popover.Root` ancestor; the popup itself
// is irrelevant to these tests, so render the content open and undecorated.
function renderContent() {
  return render(
    <PopoverPrimitive.Root open>
      <MemoryPopoverContent />
    </PopoverPrimitive.Root>,
  );
}

describe('MemoryPopoverContent', () => {
  it('shows no banner while on the default cluster', () => {
    renderContent();
    expect(screen.queryByText(/Viewing .* memory\./)).not.toBeInTheDocument();
  });

  it('shows no banner for General', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'General' }));
    expect(screen.queryByText(/Viewing .* memory\./)).not.toBeInTheDocument();
  });

  it('banners another cluster and returns to the default on Back', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: '2nd Cluster' }));
    expect(screen.getByText('Viewing 2nd Cluster’s memory.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Back to Minikube' }));
    expect(screen.queryByText(/Viewing .* memory\./)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Minikube' })).toHaveAttribute('aria-pressed', 'true');
  });
});

describe('MemoryInstructions', () => {
  it('starts empty, with a prompt to add instructions', () => {
    renderContent();
    expect(screen.getByText('No instructions added')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add instructions' })).toBeInTheDocument();
  });

  it('opens the editor on Add instructions, and saving shows the filled card', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    const textarea = screen.getByRole('textbox');
    await user.type(textarea, 'Always use traefik.');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText('Always use traefik.')).toBeInTheDocument();
    // The memory cards' icon-only edit buttons share the same accessible name.
    expect(screen.getByText('Edit')).toBeInTheDocument();
  });

  it('reopens the editor pre-filled on Edit', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    await user.type(screen.getByRole('textbox'), 'Always use traefik.');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await user.click(screen.getByText('Edit'));
    expect(screen.getByRole('textbox')).toHaveValue('Always use traefik.');
  });

  it('discards the draft on Cancel, leaving the prior state unchanged', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    await user.type(screen.getByRole('textbox'), 'Never mind this.');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText('No instructions added')).toBeInTheDocument();
  });

  it('Clear text only empties the draft, leaving the saved instructions until Save', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    await user.type(screen.getByRole('textbox'), 'Always use traefik.');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await user.click(screen.getByText('Edit'));
    await user.click(screen.getByRole('button', { name: 'Clear text' }));
    expect(screen.getByRole('textbox')).toHaveValue('');

    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByText('Always use traefik.')).toBeInTheDocument();
  });

  it('commits an empty draft on Save, clearing the saved instructions', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    await user.type(screen.getByRole('textbox'), 'Always use traefik.');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await user.click(screen.getByText('Edit'));
    await user.click(screen.getByRole('button', { name: 'Clear text' }));
    await user.click(screen.getByRole('button', { name: 'Save' }));

    expect(screen.queryByText('Always use traefik.')).not.toBeInTheDocument();
    expect(screen.getByText('No instructions added')).toBeInTheDocument();
  });

  it('disables Save on an unchanged empty draft, and Clear text once already empty', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Clear text' })).toBeDisabled();

    await user.type(screen.getByRole('textbox'), 'Always use traefik.');
    expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Clear text' })).toBeEnabled();
  });

  it('keeps instructions scoped to the cluster that saved them', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    await user.type(screen.getByRole('textbox'), 'Always use traefik.');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await user.click(screen.getByRole('button', { name: '2nd Cluster' }));
    expect(screen.getByText('No instructions added')).toBeInTheDocument();
    expect(screen.queryByText('Always use traefik.')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Back to Minikube' }));
    expect(screen.getByText('Always use traefik.')).toBeInTheDocument();
  });

  it('discards an in-progress edit when the selected cluster changes', async () => {
    const user = userEvent.setup();
    renderContent();

    await user.click(screen.getByRole('button', { name: 'Add instructions' }));
    await user.type(screen.getByRole('textbox'), 'Half-typed...');

    await user.click(screen.getByRole('button', { name: '2nd Cluster' }));
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Back to Minikube' }));
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.queryByText('Half-typed...')).not.toBeInTheDocument();
    expect(screen.getByText('No instructions added')).toBeInTheDocument();
  });
});
