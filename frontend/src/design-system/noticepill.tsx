// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Badge } from "./atoms";
import "./noticepill.css";

export type NoticePillTone = "success" | "warning" | "danger";

/**
 * A standing status the reader acts on: a soft `Badge` with its dot, as one
 * button. The press opens what the status is about, such as the trial's days
 * opening the dialog to the Margince team. A status nobody acts on stays a
 * `Badge`, which takes no click.
 */
export function NoticePill({
  tone,
  onClick,
  opensDialog = false,
  children,
}: Readonly<{
  tone: NoticePillTone;
  onClick: () => void;
  /** The press opens a dialog, which the button tells a screen reader. */
  opensDialog?: boolean;
  children: ReactNode;
}>) {
  return (
    <button
      type="button"
      className="noticepill"
      onClick={onClick}
      aria-haspopup={opensDialog ? "dialog" : undefined}
    >
      <Badge tone={tone} dot>
        {children}
      </Badge>
    </button>
  );
}
