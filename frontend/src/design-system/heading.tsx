// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { HTMLAttributes, ReactNode, Ref } from "react";
import "./heading.css";

// A heading carries two independent facts, and writing `<h2>` forces a caller to
// decide both with one keystroke: how big the words are, and where the passage
// sits in the document outline. They are not the same question — a modal's title
// is the largest thing in the modal and still an <h2> on the page — so this
// component asks them separately. `size` picks the type from the
// --fontHeading* tokens; the element follows from the size unless `as` says
// otherwise, which is the caller taking responsibility for the outline.
export type HeadingSize =
  | "xxlarge"
  | "xlarge"
  | "large"
  | "medium"
  | "small"
  | "xsmall"
  | "xxsmall";

export type HeadingElement =
  | "h1"
  | "h2"
  | "h3"
  | "h4"
  | "h5"
  | "h6"
  | "div"
  | "span";

// The element a size means when nobody says otherwise. Most headings sit where
// their size already implies their depth, so the common call names only `size`
// and still emits real structure rather than a styled <div>.
//
// THIS TABLE IS THE TWIN OF `HEADING_ELEMENT` in src/mcp-apps/bridge.ts, which
// builds the same headings for the standalone views without React to import
// this component. heading-spelling.test.ts parses both literals and fails in
// either direction on a key one side carries and the other does not.
const DEFAULT_ELEMENT: Readonly<Record<HeadingSize, HeadingElement>> = {
  xxlarge: "h1",
  xlarge: "h1",
  large: "h2",
  medium: "h3",
  small: "h4",
  xsmall: "h5",
  xxsmall: "h6",
};

export function Heading({
  size,
  as,
  className,
  children,
  ...rest
}: Readonly<
  Omit<HTMLAttributes<HTMLHeadingElement>, "children"> & {
    size: HeadingSize;
    // Overrides the element the size implies, for the case where the type and
    // the outline genuinely disagree.
    as?: HeadingElement;
    // Named explicitly because `HTMLAttributes` does not carry it: a screen
    // that moves focus to the page title after a route change needs the
    // element itself, and a heading it cannot reach is a heading it hand-rolls.
    ref?: Ref<HTMLHeadingElement>;
    children: ReactNode;
  }
>) {
  const Tag = as ?? DEFAULT_ELEMENT[size];
  // `rest` goes first so an explicit class and the size hook cannot be clobbered
  // by a caller's spread; the sheet keys off `data-size`, never off the element.
  return (
    <Tag
      {...rest}
      className={["heading", className ?? ""].filter(Boolean).join(" ")}
      data-size={size}
    >
      {children}
    </Tag>
  );
}
