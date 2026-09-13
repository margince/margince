// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Badge } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { waitingRows } from "./brief.sentence";
import { worklistLaneHref } from "./worklist.header";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

// The ROW's own sheet, because this draws the row. A surface rendering
// WorklistRow without it draws an unstyled row.
import "./worklist.css";
import "./brief.feed.css";

// The morning, as ONE ordered feed.
//
// What this replaces is the defect the whole redesign starts from: Brief drew
// "Do next" (the head of the ranked worklist) and "Focus" (the overnight
// opportunity queue) as two panels, one above the other, each with its own
// ordering. Two ranking systems gave two answers to "what first", and the rep
// had to reconcile them. The server now ranks everything once — the night's
// composite is a tie-break inside a level rather than a queue of its own — so
// the page draws that one order and nothing else.
//
// THE SERVER'S ORDER, EXACTLY. This takes a prefix and never sorts, filters or
// groups. The tie-breaks depend on a base-currency conversion and a materiality
// threshold the browser does not hold, and the contract says in as many words
// that a client renders the order it is given.
//
// THE SECTION LABEL IS A LABEL. Every row carries `brief_section`, and a badge
// is drawn when it CHANGES from the row above. That is a run-length label on an
// order somebody else decided, and it is the only thing a client may do with the
// field: partitioning the page into sections and concatenating them would be a
// second ranking, and it would disagree with the first the moment a "respond
// now" row ranked below a "move revenue" one — which is ordinary and correct,
// because a customer waiting an hour does not outrank a deal closing today.

/** At most this many rows. A morning a contact can finish, not a list. */
export const BRIEF_FEED_LIMIT = 5;

/**
 * The morning's work, in the order the server ranked it.
 */
export function BriefFeed({
  day,
  state,
  changed,
}: Readonly<{
  day: Worklist | undefined;
  state: SectionState;
  /**
   * What has MOVED since the brief was written, and where to see it.
   *
   * The count is the page's to derive and not this panel's: the same read that
   * knows when the brief was cut knows how much has changed since, and a second
   * derivation here would be a second answer to it. Absent draws no badge,
   * which is the honest state of a page that has not been told.
   */
  changed?: Readonly<{ count: number; href: string }>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const all = waitingRows(day);
  const drawn = all.slice(0, BRIEF_FEED_LIMIT);
  const rest = all.length - drawn.length;
  const restKinds = overflowByKind(all.slice(BRIEF_FEED_LIMIT));
  // WHETHER THE LABELS SAY ANYTHING. A run-length label over a single section
  // names the whole panel a second time — "Today", then "Respond now" over
  // every row in it — and a heading that is true of everything under it tells a
  // reader nothing about where they are.
  const sections = new Set(
    drawn.map((item) => item.brief_section).filter(Boolean),
  );
  // The chain reaches the FIELD, for the reason `waitingRows` does: an answer
  // that carried no summary must draw no count line rather than take the page
  // down with it. No figure is a real state — "nothing was read" — and a zero
  // over it would claim a clear morning nobody measured.
  const urgent = day?.summary?.urgent;
  return (
    // The Today panel is an ADDRESS: the head's counts link here, so the id is
    // part of the page's contract rather than decoration.
    <section id="brief-today">
      <Panel
        title={t(
          day?.scope === "team" ? "brief.feed.teamTitle" : "brief.feed.title",
        )}
        // WHAT IS ON SCREEN, out of what the day holds — the panel's own count
        // line rather than a motto. `urgent` is the SUMMARY's figure, the same
        // one the readings strip above draws, so the panel and the strip cannot
        // disagree about how much of the morning is urgent.
        sub={
          urgent === undefined
            ? undefined
            : t("brief.feed.counts", {
                items: formatNumber(all.length, locale),
                urgent: formatNumber(urgent, locale),
              })
        }
        titleAction={
          changed ? (
            <a className="entity-link" href={changed.href}>
              <Badge>
                {plural("brief.feed.changedBadge", changed.count, {
                  count: formatNumber(changed.count, locale),
                })}
              </Badge>
            </a>
          ) : undefined
        }
        footer={
          // The way to the rest. A page showing five of nineteen rows that did
          // not say where the other fourteen are has hidden them.
          day && (rest > 0 || day.next_cursor) ? (
            // The SAME cut this footer counted. `rest` comes off waitingRows,
            // which drops the approvals the Decisions deck above already draws,
            // so a bare `#/worklist` sent a rep told "11 more" to a list of 14.
            <>
              <a
                className="entity-link"
                href={worklistLaneHref("except_decisions", day.scope)}
              >
                {plural("brief.feed.rest", rest, {
                  count: formatNumber(rest, locale),
                })}
              </a>
              {restKinds.length > 1 && (
                // ONLY where the kinds differ. Over a remainder that is all one
                // thing this would name what the line above just said, which is
                // the "omit it if it repeats the cards" half of the rule.
                //
                // THE KINDS, WITHOUT COUNTS. A count beside a category label
                // composes a noun phrase, and this product's category labels
                // are singular nouns — so "2 Aufgabe" and "2 Kunde wartet" is
                // what German rendered, ungrammatically, and Vietnamese and
                // English were right only by accident of their own agreement
                // rules. Making it correct needs a plural form per category per
                // language, which is forty-two catalog entries for one caption.
                // The kinds alone answer what the reader is deciding — whether
                // the rest is more of the same or something else — and every
                // label is already a correct standalone noun in all three.
                <span className="brief-feed-rest-kinds t-caption">
                  {restKinds
                    .map(([category]) =>
                      t(`worklist.category.${category}` as MessageKey),
                    )
                    .join(" · ")}
                </span>
              )}
            </>
          ) : undefined
        }
      >
        <SurfaceState
          // A READ THAT LANDED ON NOTHING is `empty`; a read that has not
          // landed is not. Saying "nothing is waiting" over a read that failed
          // would send a rep away believing their morning was clear.
          state={state === "ready" && drawn.length === 0 ? "empty" : state}
          emptyLabel={t("brief.feed.clear")}
          loadingLabel={t("brief.feed.loading")}
        >
          {day && drawn.length > 0 && (
            // An ordered list, because the order IS the claim — and at this
            // density the element is the ONLY thing carrying it: the rows draw
            // no rank, so a bare `<ul>` here would drop the page's central
            // claim for a reader hearing it.
            <ol className="brief-feed-list">
              {drawn.map((item, index) => (
                <li key={`${item.source}-${item.id}`}>
                  {sections.size > 1 && (
                    <SectionLabel item={item} above={drawn[index - 1]} />
                  )}
                  <WorklistRow
                    item={item}
                    // ONE LINE PER ROW. Five of these open the morning and the
                    // page goes on to the rest of it, so the reader is
                    // SCANNING here — the rank is the list element's claim
                    // already, and everything a row cannot say on its line is
                    // one press away on it.
                    density="compact"
                    owner={day.scope === "team" ? (item.owner?.id ?? "") : ""}
                    onOpenEmail={setOpenEmail}
                  />
                </li>
              ))}
            </ol>
          )}
        </SurfaceState>
      </Panel>
      {/* The feed's own reader. A waiting row draws the whole message — sender,
          subject, preview, access badge — and a row that shows a reader the
          message and refuses to open it is the defect this mount removes. One
          drawer for the feed, at its level rather than inside a row, so two
          rows can never mount two dialogs.

          CARRIED ACROSS FROM "Do next", which this feed replaces. Without it
          the morning silently loses the ability to open a message it draws in
          full: worklist.row.tsx offers the opener only when the caller passes
          onOpenEmail. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
      />
    </section>
  );
}

/**
 * What the rows past the fold are, by kind, in the order they are ranked.
 *
 * A COUNT OF ROWS IS NOT A DESCRIPTION OF WORK. "14 more on the worklist" tells
 * a reader how much is left and nothing about whether it is one afternoon of
 * customer replies or fourteen system notices they can clear in a minute — and
 * those are the same number.
 *
 * First appearance decides the order, so this is the server's ranking and not a
 * second one: the kinds come out in the order the queue first reaches them,
 * which is the order the reader would meet them by scrolling.
 */
function overflowByKind(
  rows: readonly WorklistItem[],
): readonly (readonly [WorklistItem["category"], number])[] {
  const counts = new Map<WorklistItem["category"], number>();
  for (const row of rows) {
    counts.set(row.category, (counts.get(row.category) ?? 0) + 1);
  }
  return [...counts];
}

/**
 * The heading a row sits under, drawn only where it changes.
 *
 * A RUN-LENGTH LABEL on the server's order, never a grouping. The rows arrive
 * ranked; this says what part of the morning the reader has reached, and the
 * next row saying the same thing says nothing again.
 *
 * Absent where the server sent no section, which is a real state rather than a
 * gap: a category this build does not place carries none, and a heading invented
 * for it would put the row under a part of the morning nobody chose.
 *
 * WHETHER there are headings at all is the caller's: a label over a panel whose
 * rows are all one section names the panel a second time, so the feed draws
 * these only once a second part of the morning is on screen.
 */
function SectionLabel({
  item,
  above,
}: Readonly<{ item: WorklistItem; above: WorklistItem | undefined }>) {
  const t = useT();
  const section = item.brief_section;
  if (!section || (above && above.brief_section === section)) {
    return null;
  }
  return (
    <p className="brief-feed-section t-caption">
      <Badge>{t(`brief.feed.section.${section}` as const)}</Badge>
    </p>
  );
}
