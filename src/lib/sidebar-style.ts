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

// The look of the controls at the top of the left sidebar: New chat, the namespace
// picker and both search boxes. The fill and the border are the chat composer's, so
// they stay opaque on the sidebar's tint in both themes. It sits in `src/lib/` because
// `chat/` and `dashboard/` do not import each other.
//
// The `dark:` border is needed too: the outline button carries `dark:border-input`,
// which tailwind-merge only replaces with a class in the same variant chain.
export const SIDEBAR_CONTROL_CLASS = 'rounded-lg border-border bg-background shadow-xs dark:border-border dark:bg-card';
