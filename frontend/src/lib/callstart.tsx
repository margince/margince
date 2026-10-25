// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";

/** The contact a dialled number belongs to, which the logged call is filed on. */
export type CallRecord = Readonly<{
  entityType: "contact";
  entityId: string;
}>;

export type CallStart = (record: CallRecord) => void;

const CallStartContext = createContext<CallStart | null>(null);

/** Null where no log form is hosted, or the seat or record takes no activity. */
export function useCallStart(): CallStart | null {
  return useContext(CallStartContext);
}

// Always a provider, never the bare children: switching between the two would
// remount the surface under it when the seat's grants answer.
export function CallStartProvider({
  start,
  children,
}: Readonly<{ start: CallStart | null; children: ReactNode }>) {
  return (
    <CallStartContext.Provider value={start}>
      {children}
    </CallStartContext.Provider>
  );
}
