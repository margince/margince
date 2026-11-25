// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Children, type ReactNode, type Ref, type UIEventHandler } from "react";

type Band = Readonly<{ children: ReactNode; className?: string }>;

// Its first child is the `Heading` the Modal's `labelledBy` names. The drawer
// keeps this band and the foot in view while the body scrolls.
//
// `mark` is a short fact about the whole drawer (a `Badge`), drawn on the
// title's own line, after its last word.
export function DrawerHead({
  children,
  className,
  mark,
}: Band & Readonly<{ mark?: ReactNode }>) {
  const band = ["drawer-head", className ?? ""].filter(Boolean).join(" ");
  if (!mark) return <div className={band}>{children}</div>;
  const [title, ...rest] = Children.toArray(children);
  return (
    <div className={band}>
      <div className="drawer-head-line">
        {title}
        {mark}
      </div>
      {rest}
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
