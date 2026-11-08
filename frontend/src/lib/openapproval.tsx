// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";

/**
 * Opening one pending decision in the shell's decision drawer, from a record
 * page that lists it. The host is `ApprovalHost` in `app/approvalhost.tsx`.
 */
export type OpenApproval = (approvalId: string) => void;

const OpenApprovalContext = createContext<OpenApproval | null>(null);

/** The way to open a decision from here, or null when nothing hosts the drawer. */
export function useOpenApproval(): OpenApproval | null {
  return useContext(OpenApprovalContext);
}

export function OpenApprovalProvider({
  openApproval,
  children,
}: Readonly<{ openApproval: OpenApproval; children: ReactNode }>) {
  return (
    <OpenApprovalContext.Provider value={openApproval}>
      {children}
    </OpenApprovalContext.Provider>
  );
}
