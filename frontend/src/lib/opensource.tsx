// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";

/**
 * Opening the message or meeting an act was read from, from wherever the act is
 * reported.
 *
 * An activity has no page of its own, so it opens in a drawer for its kind. One
 * host in the authenticated shell owns that drawer, and a receipt only names the
 * activity. The host is `SourceHost` in `app/sourcehost.tsx`.
 */
export type OpenSource = (activityId: string) => void;

const OpenSourceContext = createContext<OpenSource | null>(null);

/** The way to open a source from here, or null when nothing hosts the drawer. */
export function useOpenSource(): OpenSource | null {
  return useContext(OpenSourceContext);
}

export function OpenSourceProvider({
  openSource,
  children,
}: Readonly<{ openSource: OpenSource; children: ReactNode }>) {
  return (
    <OpenSourceContext.Provider value={openSource}>
      {children}
    </OpenSourceContext.Provider>
  );
}
