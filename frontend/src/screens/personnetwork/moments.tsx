// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What changed about this relationship lately, and which change is a reason to
// act today.

import type { components } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { Card } from "../../design-system/atoms";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { changeSentence } from "../relationshipchange";

type RelationshipMoments = Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
>;

/**
 * momentWhyNow reads the newest change as the strip's reason to act.
 *
 * The 360 returns them newest first, so the head is the freshest thing that
 * moved. It is a heading rather than a sentence: the strip has one line, and
 * the full wording stays on the moments card where there is room for it.
 */
export function momentWhyNow(
  view: RelationshipMoments | undefined,
): string | undefined {
  const changes = view?.relationship_changes ?? [];
  // A withheld section is not an empty one. Reading a kind out of a list the
  // caller was never served would put a reason on screen from nothing.
  const withheld = (view?.sections_omitted ?? []).some(
    (section) => section === "relationship_changes",
  );
  if (withheld || changes.length === 0) {
    return undefined;
  }
  return changes[0]?.kind;
}

/**
 * MomentsCard is what changed about this relationship lately.
 *
 * It is the difference between a picture of a network and a live one: a map
 * says who is there, a moment says what moved. The sentences are the 360's
 * own, so one derived change does not get two sets of words.
 */
export function MomentsCard({ view }: Readonly<{ view: RelationshipMoments }>) {
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
    <Card
      title={t("person.network.momentsTitle")}
      sub={t("person.network.momentsSub")}
    >
      <SurfaceState state={state} emptyLabel={t("person.network.noMoments")}>
        <ul className="pn-moments">
          {changes.map((change) => (
            <li key={`${change.kind}-${change.at}`} className="pn-moment">
              <span>{changeSentence(change, t)}</span>
              <span className="pn-moment-when">
                {formatDate(change.at, locale, recordZone)}
              </span>
            </li>
          ))}
        </ul>
      </SurfaceState>
    </Card>
  );
}
