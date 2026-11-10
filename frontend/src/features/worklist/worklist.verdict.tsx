// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A saved assessment is history: it opens behind its date, so it never reads as
// the row's current reason.

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { formatDate } from "../../format/format";
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
    <Disclosure
      className="worklist-row-verdict"
      summary={
        <span className="t-caption">
          {earlierLabel(verdict, t, locale, zone)}
        </span>
      }
    >
      <p className="t-caption worklist-row-verdict-body">
        {verdict.standing && (
          <Badge>{t(`worklist.verdict.${verdict.standing}` as const)}</Badge>
        )}
        <span className="worklist-row-verdict-line">{verdict.line}</span>
      </p>
    </Disclosure>
  );
}

// The date tells a reading from this morning apart from one last week. A
// verdict the server sent without an instant names no date.
function earlierLabel(
  verdict: WorklistDealVerdict,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string {
  if (!verdict.as_of) {
    return t("worklist.verdict.believes");
  }
  return t("worklist.verdict.earlierOn", {
    date: formatDate(verdict.as_of, locale, zone),
  });
}
