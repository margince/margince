// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { EmptyState } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// A shared link, saved report or booking link that opens nothing. The screen
// names it in its own sentence, and `action` is its way on, if it has one.
export function GoneNotice({
  messageKey,
  action,
}: Readonly<{ messageKey: MessageKey; action?: ReactNode }>) {
  const t = useT();
  return (
    <EmptyState>
      <ErrorLine>{t(messageKey)}</ErrorLine>
      {action}
    </EmptyState>
  );
}
