// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Button } from "./atoms";
import "./sourcelink.css";

/**
 * SourceLink names the record a reading was taken from: the kind's glyph,
 * then the record's own name under the dotted provenance rule. The name is
 * data of any length, so it wraps and reads from the start edge. Without
 * `onOpen` there is nowhere to send the reader, and it is plain text.
 */
export function SourceLink({
  icon,
  onOpen,
  children,
}: Readonly<{
  icon: ReactNode;
  onOpen?: () => void;
  children: ReactNode;
}>) {
  const name = <span className="source-link-name">{children}</span>;
  if (!onOpen) {
    return (
      <span className="source-link">
        {icon}
        {name}
      </span>
    );
  }
  return (
    <Button
      variant="link"
      className="btn-valuelabel source-link"
      onClick={onOpen}
    >
      {icon}
      {name}
    </Button>
  );
}
