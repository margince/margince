// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel } from "../design-system/panel";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { Worklist } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

// The ROW's own sheet, because this draws the row. Its rules live with the
// component rather than with the Worklist screen that first used it, and a
// surface rendering WorklistRow without them draws an unstyled row.
import "./worklist.css";
import "./brief.donext.css";

// What waits on this rep first, on the page they open first.
//
// THE SAME ROWS THE WORKLIST DRAWS, from the same query key and the same
// component — the head of one ranked order rather than a second opinion about
// it. Home was deal-only until now: the opportunity queue below this answers
// "what is worth pursuing", which is a different question from "what is
// already waiting", and a morning that led with the second one was leading with
// the wrong half.
//
// NO SORT AND NO FILTER HERE. The order is the server's — its tie-breaks need a
// base-currency conversion and a materiality threshold the browser does not
// hold — so this takes a prefix of the queue and nothing else.

/** How many rows lead the page. */
const LEAD = 3;

/**
 * What the DECK above already answers.
 *
 * On the Worklist an approval is one row of one ranked order, and drawing it
 * there is right. On Home it is not: the decisions deck is a surface of its own
 * further up the same page, holding the same approvals and posting to the same
 * endpoint — so a row here put one decision in front of a reader twice, each
 * copy answerable, on a page whose whole claim is that it states an order once.
 *
 * Excluded by SOURCE rather than by id-matching the deck: the deck's own
 * contents are a separate read that can be pending, empty or refused while this
 * one is ready, and a rule that depended on it would draw a different page
 * depending on which read landed first.
 */
const DECK_ANSWERS = "approval";

/**
 * The head of the ranked queue, expanded.
 *
 * A prefix, deliberately: the whole queue is the Worklist's page, and a rep who
 * wants the rest has a link to it. Three is what a person acts on before the
 * morning gets away from them.
 */
export function DoNext({
  day,
  state,
}: Readonly<{ day: Worklist | undefined; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  // The optional chain reaches the FIELD, not just the payload. A worklist
  // answer that carried no queue crashed the whole page — the same mistake the
  // team gate above made, and a page that throws is a worse answer than one
  // that draws nothing.
  const waiting =
    day?.queue?.filter((item) => item.source !== DECK_ANSWERS) ?? [];
  const rows = waiting.slice(0, LEAD);
  return (
    <section id="brief-donext" aria-label={t("brief.donext.title")}>
      <Panel
        title={t("brief.donext.title")}
        sub={t("brief.donext.sub")}
        footer={
          // The way to the rest. A page showing three of eleven rows that did
          // not say where the other eight are has hidden them.
          day && waiting.length > rows.length ? (
            <a className="entity-link" href="#/worklist">
              {t("brief.donext.rest", {
                count: formatNumber(waiting.length - rows.length, locale),
              })}
            </a>
          ) : undefined
        }
      >
        <SurfaceState
          // A READ THAT LANDED ON NOTHING is `empty`; a read that has not
          // landed is not. SurfaceState will not infer it — the caller is the
          // only one who knows the difference — and saying "nothing is waiting"
          // over a read that failed would send a rep away believing their
          // morning was clear.
          state={state === "ready" && rows.length === 0 ? "empty" : state}
          emptyLabel={t("brief.donext.clear")}
          loadingLabel={t("brief.donext.loading")}
        >
          {day && rows.length > 0 && (
            // An ordered list, because the order is the claim. A screen reader
            // gets it from the element; everybody else gets it from the rank
            // the row prints.
            <ol className="brief-donext-list">
              {rows.map((item, index) => (
                <li key={item.id}>
                  <WorklistRow
                    item={item}
                    position={index + 1}
                    // The reader's OWN day. A row is handed to somebody else
                    // only from a page that is already about somebody else, and
                    // this page is about the person reading it.
                    owner=""
                    asOf={day.as_of}
                    // No pane and no filter on this surface, so the rank draws
                    // as a number rather than as a control that opens nothing.
                    // WorklistRow leaves both out when they are absent.
                  />
                </li>
              ))}
            </ol>
          )}
        </SurfaceState>
      </Panel>
    </section>
  );
}
