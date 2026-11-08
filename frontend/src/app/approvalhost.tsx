// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, Suspense, useCallback, useRef, useState } from "react";
import { lazy } from "../i18n/lazy";

const ApprovalDecisionDrawer = lazy(async () => ({
  default: (await import("../features/automations/approvaldrawer"))
    .ApprovalDecisionDrawer,
}));

import { OpenApprovalProvider } from "../lib/openapproval";

/** The shell's drawer for a decision opened from a record page under it. */
export function ApprovalHost({ children }: Readonly<{ children: ReactNode }>) {
  const [opened, setOpened] = useState<string | null>(null);
  // Focus goes back to whatever opened the drawer, which the host never sees.
  const opener = useRef<HTMLElement | null>(null);
  const openApproval = useCallback((approvalId: string) => {
    opener.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    setOpened(approvalId);
  }, []);
  return (
    <OpenApprovalProvider openApproval={openApproval}>
      {children}
      {/* Mounted only while open, so a closed drawer keeps no read. */}
      {opened !== null && (
        <Suspense fallback={null}>
          <ApprovalDecisionDrawer
            approvalId={opened}
            open
            onClose={() => setOpened(null)}
            returnFocusTo={() => opener.current}
          />
        </Suspense>
      )}
    </OpenApprovalProvider>
  );
}
