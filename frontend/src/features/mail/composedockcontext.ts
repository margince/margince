// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext } from "react";
import type { ComposeProps } from "./compose";

/**
 * One composer the app holds open. The props are the caller's at the moment
 * it opened the composer, since a composer reads its seed once, as it mounts.
 */
export type DockedCompose = Readonly<{ id: string; props: ComposeProps }>;

/**
 * The app's composer dock. Present inside the shell, where a composer outlives
 * the page that opened it; absent elsewhere (a story, a test), where the
 * composer draws in place.
 */
export type ComposeDock = Readonly<{
  show: (entry: DockedCompose) => void;
  hide: (id: string) => void;
}>;

export const ComposeDockContext = createContext<ComposeDock | null>(null);
