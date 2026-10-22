// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useMemo, useRef, useState } from "react";
import { ComposeWindow } from "./compose";
import {
  type ComposeDock,
  ComposeDockContext,
  type DockedCompose,
} from "./composedockcontext";

/**
 * Holds the composer at the app's shell, so a message being written survives
 * a move to another page. The window stays docked until it is sent or closed.
 *
 * One at a time. A second one replaces the first, whose surface is told it
 * closed. Its words are already kept, since the composer saves as it goes.
 */
export function ComposeDockProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [docked, setDocked] = useState<DockedCompose | null>(null);
  const current = useRef<DockedCompose | null>(null);
  const dock = useMemo<ComposeDock>(
    () => ({
      show: (entry) => {
        const before = current.current;
        if (before?.id === entry.id) return;
        before?.props.onClose();
        current.current = entry;
        setDocked(entry);
      },
      hide: (id) => {
        if (current.current?.id !== id) return;
        current.current = null;
        setDocked(null);
      },
    }),
    [],
  );
  return (
    <ComposeDockContext.Provider value={dock}>
      {children}
      {docked && (
        <ComposeWindow
          key={docked.id}
          {...docked.props}
          open
          onClose={() => {
            dock.hide(docked.id);
            docked.props.onClose();
          }}
        />
      )}
    </ComposeDockContext.Provider>
  );
}
