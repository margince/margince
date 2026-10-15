// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { createPortalSlot, SlotFill } from "../design-system/portalslot";
import "./pageheadaction.css";

/** The box beside the page's name; the shell draws it and hands it down. */
export const PageHeadSlot = createPortalSlot();

// The page's own verb beside its name ("Invite member"). The screen owns the
// verb's state and the shell owns the head, so the verb is portalled across.
export function PageHeadAction({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <SlotFill slot={PageHeadSlot} className="pagehead-action">
      {children}
    </SlotFill>
  );
}
