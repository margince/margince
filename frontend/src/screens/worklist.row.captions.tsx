// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ENTITY } from "../app/entity";
import { routeHash } from "../app/router";
import { formatNumber } from "../format/format";
import { translatePlural, useLocale, useT } from "../i18n";
import { subjectHref } from "./worklist.copy";
import type { WorklistItem } from "./worklist.queries";
import type { RowReadings } from "./worklist.row.compact";

// Everything a row says about itself UNDER its title: whose row it is, which
// side wrote last, when it is due, what it is worth, why it is here and what
// doing nothing costs. Beside the row rather than in it, because the row's
// file had reached the length the tree allows and these captions are one idea
// — the row's own account of itself — that reads whole on its own.

/**
 * The readings as a FRAMED row prints them: the title and the work, nothing
 * under it. The card around the row in hand on the Brief already says whose
 * row it is, which side wrote last, why it is here and where it stands in the
 * day, each once and in its own place — so the captions the queue's row prints
 * under its title would say every one of them a second time.
 *
 * Keyed on the frame rather than on the verb ORDER the card also takes: the
 * queue drawer reads its rows in that order with no card around them, and
 * there these captions are the only place those facts are said.
 */
export function shapedReadings(
  framed: boolean,
  readings: RowReadings,
): RowReadings {
  if (!framed) return readings;
  return {
    ...readings,
    when: null,
    facts: null,
    said: [],
    folded: [],
    above: null,
    consequence: null,
  };
}

// The human behind a row, as an address — the sender of a thread filed under a
// deal, the attendee a meeting is read with — through the same registry, so
// the contact's page is spelled once.
export function contactHref(contact: NonNullable<WorklistItem["contact"]>) {
  return routeHash(ENTITY.contact.route(contact.id));
}

/**
 * Whom a row is about, linked — where the row does not already say so.
 *
 * A message names its sender as text and draws no title line, so it links the
 * contact behind it here; a thread filed under a deal still came from one,
 * and the reply goes to them. Every other title names and links its own record
 * (`itemTitle`, `rowHref`), so the line repeats nothing — unless the server
 * put a contact on the row that the record is not: a task about a deal, with
 * the contact it is owed to beside it.
 */
export function aboutRecord(
  item: WorklistItem,
  named: boolean,
): RowReadings["about"] {
  const contact = item.contact;
  if (contact?.label && (named || contact.id !== item.subject?.id)) {
    return { href: contactHref(contact), label: contact.label };
  }
  if (!named) return undefined;
  const label = item.subject?.label;
  const href = subjectHref(item);
  return label && href ? { href, label } : undefined;
}

/**
 * How the silence runs both ways, on every row the server put a contact on.
 * The triage shape names them under the row itself (brief.feed.tsx) and
 * withholds this, as it withholds the about line.
 */
export function touchOf(
  item: WorklistItem,
  framed: boolean,
): NonNullable<WorklistItem["contact"]>["touch"] | undefined {
  return framed ? undefined : item.contact?.touch;
}

/**
 * How many reasons a row says before the rest go behind a tap.
 *
 * A COUNT, because the ceiling has to survive the vocabulary growing. Saying
 * only what a row contains today puts it back over the limit the next time
 * somebody adds a reason, and that contact has no way to know they did.
 *
 * Three because three still fit on ONE line at 390px. Measured 2026-09-05:
 * two reasons and three are both 19px; the fourth wraps to 37px and the sixth
 * to 56px. So the fold costs a reader nothing until the line would have taken
 * a second line anyway.
 */
export const REASONS_BEFORE_THE_FOLD = 3;

/**
 * Why this row is here — the first few said outright, the rest a tap away.
 *
 * NOTHING IS DISCARDED, which is the whole shape of this. A cap that dropped
 * the overflow was tried and abandoned (it dropped the wrong ones: `pinned`,
 * `expected_revenue` and an absorbed deal's grounds are all appended LAST
 * because they are applied late, so a head-of-list cut takes exactly the facts
 * that decided where the row sits). The reasons arrive "in the order they were
 * weighed", so the first ones are the strongest and the fold falls in the
 * right place by construction — but the rest stay reachable rather than being
 * silenced.
 *
 * The same shape the deal status card uses: first line out, remainder behind a
 * disclosure. One answer to "too many reasons", not a second one written here.
 *
 * The summary NAMES THE COUNT rather than saying "more". A reader deciding
 * whether to spend a tap wants to know if it is one more fact or four.
 *
 * WHY THIS ROW BEAT THE ONE BELOW IT is folded here too, and that is the whole
 * of where it lives now. It is one sentence of the same subject — why this row
 * is where it is — and it was drawn as a permanent third caption line under
 * every row on the page, which is a full line of height spent on a comparison
 * nobody reads until they disagree with the order. Behind this press it is one
 * tap away from the reader who does. It goes LAST and never into the summary:
 * the reasons are fragments of a dozen characters and this is a full sentence
 * with two timestamps in it, so on the line it would take the second line the
 * fold exists to save. The count covers it, because the count is what a reader
 * spends the tap on — one that named only the reasons would promise less than
 * the fold holds.
 */
function RowWhyHere({
  said,
  folded,
  above,
}: Readonly<{
  said: readonly string[];
  folded: readonly string[];
  above: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const behind = folded.length + (above ? 1 : 0);
  if (behind === 0) {
    return said.length === 0 ? null : (
      <p className="t-caption worklist-row-because">{said.join(" · ")}</p>
    );
  }
  return (
    <details className="worklist-row-because-fold">
      <summary className="t-caption worklist-row-because">
        {said.length === 0 ? (
          // Nothing is said on the line, so there is no "more" to count from
          // and the fold names itself instead — in the words the product
          // already has for this question.
          <span className="worklist-row-because-more">
            {t("worklist.verdict.rule")}
          </span>
        ) : (
          <>
            {said.join(" · ")}{" "}
            <span className="worklist-row-because-more">
              {translatePlural(locale, "worklist.because.more", behind, {
                // The reader's own notation, not String(): a count drawn for a
                // contact goes through the formatter like every other magnitude,
                // and jsx-magnitude.test.ts holds that for the whole tree.
                count: formatNumber(behind, locale),
              })}
            </span>
          </>
        )}
      </summary>
      {folded.length > 0 && (
        <p className="t-caption worklist-row-because">{folded.join(" · ")}</p>
      )}
      {above && <p className="t-caption worklist-row-above">{above}</p>}
    </details>
  );
}

/**
 * Everything the row says about itself under the title, in the order a reader
 * needs it.
 *
 * When it happens, what it is worth, why it is ranked where it is, and what
 * doing nothing costs. Each is absent when the server sent nothing for it — a
 * caption drawn empty is a line of furniture the reader has to look past on
 * every row.
 *
 * TWO LINES AND NOT FIVE. The row printed the moment, the figures, the reasons,
 * the consequence and the comparison as five stacked captions, which is five
 * lines of 12px grey under every title on the page — and a reader scanning a
 * queue reads the first of them and skips the rest. The facts that are
 * FRAGMENTS ("due 15:00", "€40k", "waiting 4 days · nobody owns it") now read
 * as one dot-separated line at every width, in the same order and still as
 * separate elements, so a screen reader still meets three facts and only the
 * line breaks between them go. What keeps a line of its own is the sentence a
 * reader is meant to stop at: what it costs to do nothing.
 *
 * Together in one component because they are one idea — the row's own account
 * of itself — and because the row's function had reached the complexity the
 * linter allows, which is a fair reading of how much a contact can hold at once.
 */
export function RowCaptions({
  about,
  touch,
  when,
  facts,
  said,
  folded,
  consequence,
  above,
}: Readonly<{
  about?: RowReadings["about"];
  touch: RowReadings["touch"];
  when: string | null;
  facts: string | null;
  said: readonly string[];
  folded: readonly string[];
  consequence: string | null;
  above: string | null;
}>) {
  return (
    <>
      {/* When it starts, or when it is due, FIRST — it is the fact the reasons
          beside it are about: "starting shortly" explains a rank, and this says
          what time. */}
      <div className="worklist-row-facts-line">
        {/* WHO it is with, first: a rep answering a row asks whose row it is
            before how long it has waited. */}
        {about && (
          <p className="t-caption worklist-row-about">
            <a className="entity-link" href={about.href}>
              {about.label}
            </a>
          </p>
        )}
        {when && <p className="t-caption worklist-row-when">{when}</p>}
        {facts && <p className="t-caption worklist-row-facts">{facts}</p>}
        <RowWhyHere said={said} folded={folded} above={above} />
      </div>
      {/* Which side wrote last, on a line of its own under the fragments: two
          dated facts read as one statement about the relationship, and the
          question a rep answers a waiting row with. */}
      {touch.length > 0 && (
        <p className="t-caption worklist-row-touch">
          {touch.map((fact) => (
            <span key={fact.term} className="worklist-row-touch-fact">
              <span>{fact.term}</span>{" "}
              <span className="t-num">{fact.value}</span>
            </span>
          ))}
        </p>
      )}
      {/* What it costs to do nothing. The question a queue exists to answer,
          and the one the lane feed had no field for. */}
      {consequence && (
        <p className="t-caption worklist-row-consequence">{consequence}</p>
      )}
    </>
  );
}
