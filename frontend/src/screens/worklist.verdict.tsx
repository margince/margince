// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How the deal behind a row is STANDING, drawn beside the step that acts on it.
//
// The row already said what to do. This says what the reader is walking into —
// the judgement the deal page prints, carried onto the row so the reader does
// not have to open that page to find it.
//
// THE LABEL SAYS THIS IS A READING, AND THAT IS THE WHOLE POINT OF DRAWING IT.
// The server sends two kinds of line and both are readings:
//
//   deal_status   — the deal's own card, written from its timeline and cited.
//   brief_finding — the night's grounded finding about this deal.
//
// A row that arrives with NO verdict is not a gap and gets nothing from here.
// Its typed reasons are drawn underneath by RowCaptions, and those are the
// deterministic explanation — computed from records with no model in the path.
// Presenting them under "Margince believes" would be the lie this component
// exists to prevent, which is why the caption block and this one are separate
// and why the contract has no third `source` member for that case.
//
// `standing` is absent on a brief finding, which is prose about the deal rather
// than one of the four calls. The badge is drawn only where the server sent a
// word, never invented from the sentence.

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { formatDateTime } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";

/**
 * The four standings, and the tone each is drawn in.
 *
 * `cold` takes NO tone deliberately: the palette's tones each carry an
 * instruction — success is fine, warn is act soon, danger is act now — and a
 * deal treated as lost is none of those. Drawn plain, it reads as the statement
 * of fact it is rather than as a fourth thing competing for the same attention.
 */
const STANDING_TONE = {
  live: "success",
  drifting: "warn",
  blocked: "danger",
  cold: undefined,
} as const;

/**
 * Draws one row's verdict, or nothing when the server sent none.
 *
 * Nothing is the ordinary case on most rows: no card is cached for the deal and
 * the night did not surface it. The row is still fully explained by the reasons
 * below it, so this draws no placeholder and no empty state.
 */
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
        <Badge tone={STANDING_TONE[verdict.standing]}>
          {t(`worklist.verdict.${verdict.standing}` as const)}
        </Badge>
      )}
      <span className="worklist-row-verdict-source">
        {t("worklist.verdict.believes")}
      </span>
      <span className="worklist-row-verdict-line">{verdict.line}</span>
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
    <span className="worklist-row-verdict-when">
      {t("worklist.verdict.asOf", {
        when: formatDateTime(verdict.as_of, locale, zone),
      })}
    </span>
  );
}
