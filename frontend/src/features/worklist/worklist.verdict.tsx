// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Saved assessments keep their date beside independently checked current actions.

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { formatDateTime } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";

type WorklistDealVerdict = components["schemas"]["WorklistDealVerdict"];

export function VerdictLine({
  verdict,
  zone,
}: {
  verdict: WorklistDealVerdict | undefined;
  zone: string;
}) {
  const t = useT();
  const { locale } = useLocale();
  if (!verdict) {
    return null;
  }
  return (
    <p className="t-caption worklist-row-verdict">
      {verdict.standing && (
        <Badge>{t(`worklist.verdict.${verdict.standing}` as const)}</Badge>
      )}
      <span>{t("worklist.verdict.believes")}</span>{" "}
      <span className="worklist-row-verdict-line">{verdict.line}</span>{" "}
      {asOfText(verdict, t, locale, zone)}
    </p>
  );
}

/**
 * When the reading behind this line was taken.
 *
 * Drawn because a cached card can be days old and the row does not otherwise
 * say so — a rep acting on "they went quiet" wants to know whether that was read
 * this morning or last week. Absent when the server sent no instant.
 */
function asOfText(
  verdict: WorklistDealVerdict,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
) {
  if (!verdict.as_of) {
    return null;
  }
  return (
    <span>
      {t("worklist.verdict.asOf", {
        when: formatDateTime(verdict.as_of, locale, zone),
      })}
    </span>
  );
}
