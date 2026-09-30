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

import { act, screen, waitFor } from '@testing-library/react';
import { createRootRoute, createRoute, Outlet } from '@tanstack/react-router';
import { describe, expect, it } from 'vitest';

import { renderWithRouter } from '@/test-utils';
import { HomeButton } from './home-button';

// Helpers -------------------------------------------------------------

// The button plus its destination and one other page, so navigation can be
// observed by which page renders.
function buildTree() {
  const root = createRootRoute({
    component: () => (
      <>
        <HomeButton />
        <Outlet />
      </>
    ),
  });
  const chat = createRoute({
    getParentRoute: () => root,
    path: '/chat',
    component: () => <div>chat-page</div>,
  });
  const dashboard = createRoute({
    getParentRoute: () => root,
    path: '/dashboard',
    component: () => <div>dashboard-page</div>,
  });
  return root.addChildren([chat, dashboard]);
}

// Tests ---------------------------------------------------------------

describe('HomeButton', () => {
  it('links home', async () => {
    await renderWithRouter(buildTree(), '/dashboard');
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('href', '/chat');
  });

  it('navigates home when clicked', async () => {
    await renderWithRouter(buildTree(), '/dashboard');
    expect(screen.getByText('dashboard-page')).toBeInTheDocument();

    act(() => {
      screen.getByRole('link', { name: 'Home' }).click();
    });

    await waitFor(() => expect(screen.getByText('chat-page')).toBeInTheDocument());
  });

  it('marks itself current while home is the open route', async () => {
    await renderWithRouter(buildTree(), '/chat');
    expect(screen.getByRole('link', { name: 'Home' })).toHaveAttribute('aria-current', 'page');
  });
});
