// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Badge } from "../design-system/atoms";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { waitingRows } from "./brief.sentence";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

// The ROW's own sheet, because this draws the row. A surface rendering
// WorklistRow without it draws an unstyled row.
import "./worklist.css";
import "./brief.feed.css";

// The morning, as ONE ordered feed.
//
// What this replaces is the defect the whole redesign starts from: Home drew
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

/** At most this many cards. A morning a person can finish, not a list. */
const FEED = 8;

/**
 * The morning's work, in the order the server ranked it.
 */
export function BriefFeed({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const all = waitingRows(day);
  const drawn = all.slice(0, FEED);
  const rest = all.length - drawn.length;
  return (
    <section id="brief-feed" aria-label={t("brief.feed.title")}>
      <Panel
        title={t("brief.feed.title")}
        sub={t("brief.feed.sub")}
        footer={
          // The way to the rest. A page showing eight of nineteen rows that did
          // not say where the other eleven are has hidden them.
          day && rest > 0 ? (
            <a className="entity-link" href="#/worklist">
              {t("brief.feed.rest", { count: formatNumber(rest, locale) })}
            </a>
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
            // An ordered list, because the order IS the claim. A screen reader
            // gets it from the element; everybody else gets it from the rank
            // the row prints.
            <ol className="brief-feed-list">
              {drawn.map((item, index) => (
                <li key={item.id}>
                  <SectionLabel item={item} above={drawn[index - 1]} />
                  <WorklistRow
                    item={item}
                    position={index + 1}
                    // The reader's OWN day. A row is handed to somebody else
                    // only from a page that is already about somebody else,
                    // and this page is about the person reading it.
                    owner=""
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
 * The heading a row sits under, drawn only where it changes.
 *
 * A RUN-LENGTH LABEL on the server's order, never a grouping. The rows arrive
 * ranked; this says what part of the morning the reader has reached, and the
 * next row saying the same thing says nothing again.
 *
 * Absent where the server sent no section, which is a real state rather than a
 * gap: a category this build does not place carries none, and a heading invented
 * for it would put the row under a part of the morning nobody chose.
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
