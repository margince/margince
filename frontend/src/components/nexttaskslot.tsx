// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";

// Record pages draw the shell's next-task control without importing the Worklist.
const NextTask = createContext<ReactNode>(null);

export const NextTaskProvider = NextTask.Provider;

/** The way on to the next Worklist task, or nothing where no shell hosts one. */
export function NextTaskSlot() {
  return useContext(NextTask);
}
