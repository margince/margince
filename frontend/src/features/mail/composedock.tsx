// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ReactNode,
  Suspense,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { BesidePage } from "../../components/unsaved";
import { lazy } from "../../i18n/lazy";
import {
  type ComposeDock,
  ComposeDockContext,
  type DockedCompose,
} from "./composedockcontext";
import type { ComposeProps } from "./composeprops";

// A page that offers to write never carries the window's code; it arrives
// when someone opens the composer.
export function loadComposeWindow() {
  return import("./composewindow");
}

const ComposeWindow = lazy(async () => ({
  default: (await loadComposeWindow()).ComposeWindow,
}));

/**
 * The composer, as every surface opens it. Inside the app's shell it hands
 * itself to the dock, so the message outlives the page that opened it.
 * Anywhere else (a story, a test) it draws in place.
 */
export function ComposeModal(props: ComposeProps) {
  const dock = useContext(ComposeDockContext);
  if (!dock) {
    return (
      <Suspense fallback={null}>
        <ComposeWindow {...props} />
      </Suspense>
    );
  }
  return <DockedLauncher dock={dock} props={props} />;
}

// Opens and shuts this surface's composer in the dock. A surface that goes
// away with the composer open leaves it docked: that is what the dock is for.
function DockedLauncher({
  dock,
  props,
}: Readonly<{ dock: ComposeDock; props: ComposeProps }>) {
  const id = useId();
  const latest = useRef(props);
  useEffect(() => {
    latest.current = props;
  });
  const { open } = props;
  useEffect(() => {
    if (open) {
      dock.show({ id, props: latest.current });
    } else {
      dock.hide(id);
    }
  }, [open, dock, id]);
  return null;
}

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
        <BesidePage>
          <Suspense fallback={null}>
            <ComposeWindow
              key={docked.id}
              {...docked.props}
              open
              onClose={() => {
                dock.hide(docked.id);
                docked.props.onClose();
              }}
            />
          </Suspense>
        </BesidePage>
      )}
    </ComposeDockContext.Provider>
  );
}
