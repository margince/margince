// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Context, createContext, type ReactNode, useContext } from "react";
import { createPortal } from "react-dom";

/** The element a host draws for content written further down the tree. */
export type PortalSlot = Context<HTMLElement | null>;

export function createPortalSlot(): PortalSlot {
  return createContext<HTMLElement | null>(null);
}

// No element yet, or no host at all: the content draws where it is written, so
// it never vanishes with the frame it expected.
export function SlotFill({
  slot,
  className,
  children,
}: Readonly<{
  slot: PortalSlot;
  /** The box the content stands in when it draws in place. */
  className: string;
  children: ReactNode;
}>) {
  const element = useContext(slot);
  if (element === null) {
    return <div className={className}>{children}</div>;
  }
  return createPortal(children, element);
}
