// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  createContext,
  type ReactNode,
  useContext,
  useMemo,
  useState,
} from "react";
import { createPortal } from "react-dom";
import "./pageheadaction.css";

// The page's own verb beside its name ("Invite member"). The screen owns the
// verb's state and the shell owns the head, so the verb is portalled across.

type SlotState = Readonly<{
  slot: HTMLElement | null;
  setSlot: (slot: HTMLElement | null) => void;
}>;

const PageHeadSlot = createContext<SlotState | null>(null);

export function PageHeadActionProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  const value = useMemo(() => ({ slot, setSlot }), [slot]);
  return (
    <PageHeadSlot.Provider value={value}>{children}</PageHeadSlot.Provider>
  );
}

/** Where the page head takes its verb. Empty, it takes no room. */
export function PageHeadActionSlot() {
  const state = useContext(PageHeadSlot);
  return <div className="pagehead-action" ref={state?.setSlot} />;
}

// With no head at all (a story, a screen test) the verb draws where it is
// written. Under the shell it waits one commit for the head's slot instead of
// flashing inside the page.
export function PageHeadAction({
  children,
}: Readonly<{ children: ReactNode }>) {
  const state = useContext(PageHeadSlot);
  if (state === null) {
    return <div className="pagehead-action">{children}</div>;
  }
  return state.slot ? createPortal(children, state.slot) : null;
}
