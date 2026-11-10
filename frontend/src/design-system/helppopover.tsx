// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Info } from "lucide-react";
import type { ReactNode } from "react";
import { Popover } from "./popover";
import "./helppopover.css";

/**
 * HelpPopover is the bare ⓘ beside a label or a sentence that opens a short
 * explanation. `label` names the trigger for a screen reader, since the glyph
 * says nothing on its own.
 */
export function HelpPopover({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <Popover
      className="help-popover"
      label={
        <>
          <Info aria-hidden />
          <span className="sr-only">{label}</span>
        </>
      }
    >
      {children}
    </Popover>
  );
}
