// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";
import type { components } from "../api/schema";

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

/** What a surface knows about a source before it is opened. */
export type SourceRef = Readonly<{
  id: string | null | undefined;
  kind: components["schemas"]["Activity"]["kind"];
  contentState?: "available" | "withheld" | null;
}>;

/** How a surface opens one source, or undefined when it draws no control. */
export type SourceOpenerFor = (ref: SourceRef) => (() => void) | undefined;

/**
 * The one opener every surface that shows a source uses, so each opens it
 * whole in the shell's drawer and closing returns to that surface.
 *
 * A withheld source opens nothing: the drawer would only repeat that it is
 * withheld, and a control that leads there claims something can be read. No
 * host (a bare story or test) also opens nothing. `onOpenEmail` lets a page
 * that owns its own message drawer keep it for messages.
 */
export function useSourceOpener(
  onOpenEmail?: (activityId: string) => void,
): SourceOpenerFor {
  const openSource = useOpenSource();
  return (ref) => {
    const id = ref.id;
    if (!id || ref.contentState === "withheld") return undefined;
    const open = (ref.kind === "email" && onOpenEmail) || openSource;
    return open ? () => open(id) : undefined;
  };
}
