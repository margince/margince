// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown, ChevronUp } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import "./composethread.css";

// The column the draft answers from. composethread.css draws it beside the
// draft where the drawer holds both columns, and behind this toggle where not.
// Not Disclosure: a closed <details> hides the column where it must stand open.
export function ConversationFold({
  choosing,
  children,
}: Readonly<{ choosing: boolean; children: ReactNode }>) {
  const t = useT();
  const columnId = useId();
  const [open, setOpen] = useState(false);
  const shown = choosing ? "compose.choicesHide" : "compose.threadHide";
  const hidden = choosing ? "compose.choicesShow" : "compose.threadShow";
  return (
    <div className="compose-fold" data-open={open}>
      <Button
        className="compose-fold-toggle"
        aria-expanded={open}
        aria-controls={columnId}
        onClick={() => setOpen((was) => !was)}
      >
        {open ? (
          <ChevronUp aria-hidden="true" />
        ) : (
          <ChevronDown aria-hidden="true" />
        )}
        {t(open ? shown : hidden)}
      </Button>
      <div id={columnId} className="compose-fold-column">
        {children}
      </div>
    </div>
  );
}
