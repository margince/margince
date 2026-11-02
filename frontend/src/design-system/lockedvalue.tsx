// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Lock } from "lucide-react";
import { createContext, type ReactNode, useContext } from "react";
import { useT } from "../i18n";
import { Popover } from "./popover";
import "./lockedvalue.css";

/**
 * Why the whole record around a value is locked, as its banner says it. A
 * value with this same reason draws no lock, because the banner said it once.
 */
export const RecordRefusalContext = createContext<string | undefined>(
  undefined,
);

/**
 * A value the reader may not change. When the reason is the field's own, a
 * lock sits beside the value. A press or a settled pointer opens the reason.
 */
export function LockedValue({
  label,
  reason,
  className,
  testId,
  children,
}: Readonly<{
  label: string;
  reason?: string;
  className?: string;
  testId?: string;
  children: ReactNode;
}>) {
  const t = useT();
  const recordRefusal = useContext(RecordRefusalContext);
  if (!reason || reason === recordRefusal) {
    return (
      <span className={className} data-testid={testId}>
        {children}
      </span>
    );
  }
  return (
    <span className="locked-value">
      <span className={className} data-testid={testId}>
        {children}
      </span>
      <Popover
        className="locked-value-mark"
        onHover
        align="end"
        label={
          <Lock size={12} aria-label={t("field.lockedWhy", { field: label })} />
        }
      >
        {reason}
      </Popover>
    </span>
  );
}
