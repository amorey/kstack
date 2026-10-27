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

import { useState } from 'react';

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { MemoryRail } from './rail';
import type { Scope } from './rail';

// The rail is controlled; this gives the selection somewhere to live.
function Rail({ clusters = ['Minikube', '2nd Cluster'] }: { clusters?: string[] }) {
  const [selected, setSelected] = useState<Scope>({ kind: 'cluster', name: clusters[0] });
  return <MemoryRail clusters={clusters} selected={selected} onSelect={setSelected} />;
}

const row = (name: string) => screen.getByRole('button', { name });

describe('MemoryRail', () => {
  it('starts with the cluster group open and the default cluster selected', () => {
    render(<Rail />);
    expect(screen.getByRole('button', { name: 'Cluster' })).toHaveAttribute('aria-expanded', 'true');
    expect(row('Minikube')).toHaveAttribute('aria-pressed', 'true');
  });

  it('collapses the cluster group without losing the selection', async () => {
    const user = userEvent.setup();
    render(<Rail />);

    await user.click(screen.getByRole('button', { name: 'Cluster' }));
    expect(screen.queryByRole('button', { name: 'Minikube' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Cluster' }));
    expect(row('Minikube')).toHaveAttribute('aria-pressed', 'true');
  });

  it('selects one row at a time, cluster or General', async () => {
    const user = userEvent.setup();
    render(<Rail />);

    await user.click(row('2nd Cluster'));
    expect(row('2nd Cluster')).toHaveAttribute('aria-pressed', 'true');
    expect(row('Minikube')).toHaveAttribute('aria-pressed', 'false');

    await user.click(row('General'));
    expect(row('General')).toHaveAttribute('aria-pressed', 'true');
    expect(row('2nd Cluster')).toHaveAttribute('aria-pressed', 'false');
  });

  it('names each row in full in its title, for a name cut short', () => {
    render(<Rail />);
    expect(row('2nd Cluster')).toHaveAttribute('title', '2nd Cluster');
  });

  it('keeps a context named General apart from the General row', async () => {
    const user = userEvent.setup();
    render(<Rail clusters={['General', 'Minikube']} />);
    const [cluster, general] = screen.getAllByRole('button', { name: 'General' });
    expect(cluster).toHaveAttribute('aria-pressed', 'true');
    expect(general).toHaveAttribute('aria-pressed', 'false');

    await user.click(general);
    expect(cluster).toHaveAttribute('aria-pressed', 'false');
    expect(general).toHaveAttribute('aria-pressed', 'true');
  });
});
