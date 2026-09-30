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

// Home button at the left of the app bar. A router link wearing the app bar's
// button styling, so it is a real destination and marks itself current like the
// rest of the app's navigation. The styling is `appBarButtonClass`, not the
// `AppBarButton` element: that element's primitive insists on either a native
// `<button>` or `role="button"` on whatever it renders, and an anchor wearing
// the button role is no longer a link.
import { House } from 'lucide-react';
import { Link } from '@tanstack/react-router';

import { appBarButtonClass } from '@/components/widgets/app-bar-button';

export function HomeButton() {
  return (
    <Link to="/chat" aria-label="Home" className={appBarButtonClass()}>
      <House aria-hidden />
    </Link>
  );
}
