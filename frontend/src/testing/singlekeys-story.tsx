// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect } from "react";
import { setSingleKeyShortcutsOn } from "../lib/shortcuts";

/** Turns single-key shortcuts back on as the story leaves: the choice is one
 *  per browser, and left off it would follow every story rendered after. */
export function RestoreSingleKeys({
  children,
}: Readonly<{ children: ReactNode }>) {
  useEffect(() => () => setSingleKeyShortcutsOn(true), []);
  return <>{children}</>;
}
