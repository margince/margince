// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode, Ref, UIEventHandler } from "react";

type Band = Readonly<{ children: ReactNode; className?: string }>;

// Its first child is the `Heading` the Modal's `labelledBy` names. The drawer
// keeps this band and the foot in view while the body scrolls.
export function DrawerHead({ children, className }: Band) {
  return (
    <div className={["drawer-head", className ?? ""].filter(Boolean).join(" ")}>
      {children}
    </div>
  );
}

export function DrawerBody({
  children,
  className,
  ref,
  onScroll,
}: Readonly<{
  children: ReactNode;
  className?: string;
  ref?: Ref<HTMLDivElement>;
  onScroll?: UIEventHandler<HTMLDivElement>;
}>) {
  return (
    <div
      ref={ref}
      className={["drawer-body", className ?? ""].filter(Boolean).join(" ")}
      onScroll={onScroll}
    >
      {children}
    </div>
  );
}

export function DrawerFoot({ children, className }: Band) {
  return (
    <div className={["drawer-foot", className ?? ""].filter(Boolean).join(" ")}>
      {children}
    </div>
  );
}
