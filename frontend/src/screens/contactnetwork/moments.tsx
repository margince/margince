// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed about this relationship lately, and which change is a reason to
// act today.

import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { changeSentence } from "../relationshipchange";

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
 * MomentsPanel is what changed about this relationship lately.
 *
 * It is the difference between a picture of a network and a live one: a map
 * says who is there, a moment says what moved. The sentences are the 360's
 * own, so one derived change does not get two sets of words.
 */
export function MomentsPanel({
  view,
}: Readonly<{ view: RelationshipMoments }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const changes = view.relationship_changes ?? [];
  // The section is withholdable. A reader without the grant is served no
  // changes, and "nothing has moved" would be a fact the page does not have.
  const withheld = (view.sections_omitted ?? []).some(
    (section) => section === "relationship_changes",
  );
  const state = withheld
    ? ("withheld" as const)
    : changes.length === 0
      ? ("empty" as const)
      : ("ready" as const);
  return (
    <Panel title={t("contact.network.momentsTitle")}>
      {/* Where the movements are read FROM, which is what makes them evidence
          rather than a summary — too long for the head's one line, and the
          half that would truncate is the half naming the source. */}
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
            {changes.map((change, index) => (
              <li key={`${change.kind}-${change.at}`} className="pn-moment">
                <time className="pn-moment-when t-caption" dateTime={change.at}>
                  {formatDate(change.at, locale, recordZone)}
                </time>
                <p>
                  {changeSentence(change, t)}
                  {/* The head is what the strip's "why now" slot was read
                      from, and is flagged here so the two agree on which
                      change that was. */}
                  {index === 0 ? (
                    <Badge tone="accent">
                      {t("contact.intro.stripWhyNow")}
                    </Badge>
                  ) : null}
                </p>
              </li>
            ))}
          </ol>
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
