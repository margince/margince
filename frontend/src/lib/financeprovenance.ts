// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";

// Where a finance figure comes from, for a caption. A source's key is not a
// name, so a source that declares none is "your accounting system".
export function financeProvenance(
  t: Translator,
  name: string | null | undefined,
  syncedWhen: string | null | undefined,
): string {
  if (name) {
    return syncedWhen
      ? t("finance.syncedFrom", { provider: name, when: syncedWhen })
      : t("finance.fromNeverSynced", { provider: name });
  }
  return syncedWhen
    ? t("finance.syncedFromAccounting", { when: syncedWhen })
    : t("finance.fromAccountingNeverSynced");
}
