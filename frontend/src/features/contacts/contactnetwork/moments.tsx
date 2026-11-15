// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed about this relationship lately, which change is a reason to
// act today, and the earlier asks about the contact, all on one dated line.

import type { components } from "../../../api/schema";
import { Badge } from "../../../design-system/atoms";
import { Panel, PanelBody } from "../../../design-system/panel";
import { SurfaceState } from "../../../design-system/surfacestate";
import { formatDate } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { useViewerZone } from "../../../lib/viewerzone";
import { AskLine, askSettledAt } from "../introasks";
import type { IntroRequest } from "../introrequests";
import { changeSentence } from "../relationshipchange";

type Change = components["schemas"]["ContactRelationshipChange"];

type Moment =
  | Readonly<{ kind: "change"; at: string; change: Change; latest: boolean }>
  | Readonly<{ kind: "ask"; at: string; ask: IntroRequest }>;

type RelationshipMoments = Pick<
  components["schemas"]["Contact360"],
  "relationship_changes" | "sections_omitted"
>;

/**
 * latestChange reads the newest change, and whether there was one to read.
 *
 * The 360 returns them newest first, so the head is the freshest thing that
 * moved. The CHANGE rather than a sentence about it: the strip writes the
 * change in its own two-line vocabulary, and the panel below writes the
 * sentence — one derived change, two surfaces, neither borrowing the other's
 * shape.
 *
 * `withheld` travels with it because a withheld section is not an empty one,
 * and a strip that cannot tell them apart reports a refusal as "nothing new".
 */
export function latestChange(view: RelationshipMoments | undefined): Readonly<{
  change?: components["schemas"]["ContactRelationshipChange"];
  withheld: boolean;
}> {
  const withheld = (view?.sections_omitted ?? []).some(
    (section) => section === "relationship_changes",
  );
  return {
    change: withheld ? undefined : view?.relationship_changes?.[0],
    withheld,
  };
}

/**
 * timeline puts the changes and the settled asks on one line, newest first.
 * The head change keeps its flag, because the strip's "why now" reads it.
 */
function timeline(
  changes: readonly Change[],
  asks: readonly IntroRequest[],
): Moment[] {
  const moments: Moment[] = [
    ...changes.map((change, index) => ({
      kind: "change" as const,
      at: change.at,
      change,
      latest: index === 0,
    })),
    ...asks.map((ask) => ({
      kind: "ask" as const,
      at: askSettledAt(ask),
      ask,
    })),
  ];
  return moments.sort((a, b) => Date.parse(b.at) - Date.parse(a.at));
}

/**
 * MomentsPanel is what moved in this relationship lately, earlier asks
 * included.
 *
 * A map says who is there, a moment says what moved. The change sentences are
 * the 360's own, so one derived change does not get two sets of words.
 */
export function MomentsPanel({
  view,
  asks = [],
}: Readonly<{
  view: RelationshipMoments;
  // Settled asks only: an open one keeps its verbs in the asks panel above.
  asks?: readonly IntroRequest[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  // The changes section is withholdable. A reader without the grant is served
  // none, and "nothing has moved" would be a fact the page does not have.
  const withheld = (view.sections_omitted ?? []).some(
    (section) => section === "relationship_changes",
  );
  const moments = timeline(
    withheld ? [] : (view.relationship_changes ?? []),
    asks,
  );
  const state =
    moments.length > 0
      ? ("ready" as const)
      : withheld
        ? ("withheld" as const)
        : ("empty" as const);
  return (
    <Panel title={t("contact.network.momentsTitle")}>
      {/* Where the movements are read from, which makes them evidence rather
          than a summary. */}
      <PanelBody>
        <p className="t-sub">{t("contact.network.momentsSub")}</p>
      </PanelBody>
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("contact.network.noMoments")}
          loadingLabel={t("contact.network.title")}
        >
          <ol className="pn-moments">
            {moments.map((moment) => (
              <li
                key={
                  moment.kind === "ask"
                    ? `ask-${moment.ask.id}`
                    : `${moment.change.kind}-${moment.at}`
                }
                className="pn-moment"
              >
                <time className="pn-moment-when t-caption" dateTime={moment.at}>
                  {formatDate(moment.at, locale, readerZone)}
                </time>
                <p>
                  {moment.kind === "ask" ? (
                    <AskLine ask={moment.ask} />
                  ) : (
                    <>
                      {changeSentence(moment.change, t)}
                      {moment.latest ? (
                        <Badge tone="accent">
                          {t("contact.intro.stripWhyNow")}
                        </Badge>
                      ) : null}
                    </>
                  )}
                </p>
              </li>
            ))}
          </ol>
        </SurfaceState>
        {/* Asks drawn while changes are withheld must not read as the whole
            story, so the refusal still shows under them. */}
        {withheld && moments.length > 0 ? (
          <SurfaceState
            state="withheld"
            emptyLabel={t("contact.network.noMoments")}
            loadingLabel={t("contact.network.title")}
          >
            {null}
          </SurfaceState>
        ) : null}
      </PanelBody>
    </Panel>
  );
}
