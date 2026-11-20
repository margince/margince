// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { translate, useLocale, useT } from "../../i18n";

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
  "thread",
  "sole_open_deal",
  "stakeholder",
  "named_stakeholder",
  "ambiguous",
] as const;

/**
 * The copy namespace a unit's translations live in: `crm-hello` is
 * `extCrmHello.`. Core cannot import the generator's rule, so this restates it,
 * and the test beside it holds the two together.
 */
export function unitCopyPrefix(unit: string): string {
  const camel = unit
    .split("-")
    .filter((part) => part !== "")
    .map((part) =>
      /^\d/.test(part) ? `_${part}` : part[0].toUpperCase() + part.slice(1),
    )
    .join("");
  return `ext${camel}.`;
}

/** A unit's own wording for a key, or the key itself when it has none. */
function unitWords(
  locale: Parameters<typeof translate>[0],
  unit: string,
  key: string,
): string {
  const full = `${unitCopyPrefix(unit)}${key}` as `ext${string}`;
  const words = translate(locale, full);
  return words === full ? "" : words;
}

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
