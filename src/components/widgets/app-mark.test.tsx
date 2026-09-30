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

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { AppMark } from './app-mark';

describe('AppMark', () => {
  afterEach(cleanup);

  it('renders the letter decoratively at the size given', () => {
    render(<AppMark size={48} />);
    const mark = screen.getByTestId('app-mark');
    expect(mark).toHaveTextContent('K');
    expect(mark).toHaveAttribute('aria-hidden', 'true');
    expect(mark).toHaveClass('size-12');
  });

  it('draws the sidebar size', () => {
    render(<AppMark size={24} />);
    expect(screen.getByTestId('app-mark')).toHaveClass('size-6');
  });
});
