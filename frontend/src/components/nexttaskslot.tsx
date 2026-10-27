// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";

// A record page draws the way on to the next Worklist task without importing
// the Worklist: the shell mounts the one that reads it, and this draws it.
const NextTask = createContext<ReactNode>(null);

export const NextTaskProvider = NextTask.Provider;

/** The way on to the next Worklist task, or nothing where no shell hosts one. */
export function NextTaskSlot() {
  return useContext(NextTask);
}
