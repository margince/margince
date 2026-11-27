// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useLocale, useT } from "../../i18n";
import { unitWords } from "../../lib/extensions";

// Who filed a message on its deal, and why. The actor is `core` or the name of
// an extension. An extension's reason is a key it declared, and its label is in
// that extension's own translations.

type Filing = NonNullable<components["schemas"]["CaptureTraceFiling"]>;

const EXT_RESULTS = [
  "linked",
  "none",
  "ambiguous",
  "parties_changed",
  "failed",
  "ignored",
  "moved",
  "retracted",
] as const;

const CORE_BASES = [
  "project",
  "chosen_thread",
  "thread",
  "sole_open_deal",
  "stakeholder",
  "named_stakeholder",
  "ambiguous",
] as const;

export function FilingLine({ filing }: Readonly<{ filing: Filing }>) {
  const t = useT();
  const { locale } = useLocale();
  if (filing.actor === "core") {
    const basis = CORE_BASES.find((known) => known === filing.reason);
    const reason = basis ? t(`captureActivity.filing.basis.${basis}`) : "";
    if (filing.result === "moved") {
      return <>{t("captureActivity.filing.core.moved", { reason })}</>;
    }
    if (filing.result === "retracted") {
      return <>{t("captureActivity.filing.core.retracted", { reason })}</>;
    }
    return <>{t("captureActivity.filing.core", { reason })}</>;
  }
  const result = EXT_RESULTS.find((known) => known === filing.result);
  if (!result) {
    return null;
  }
  const unit = unitWords(locale, filing.actor, "title") || filing.actor;
  const reason = filing.reason
    ? unitWords(locale, filing.actor, `captureReason.${filing.reason}`) ||
      filing.reason
    : "";
  return <>{t(`captureActivity.filing.ext.${result}`, { unit, reason })}</>;
}
