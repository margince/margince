// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who is on this deal — ONE card.
//
// Until this, the same seats were drawn three times on one page: the committee
// map named them under its picture, a rail panel listed them again with their
// engagement, and the stakeholders table listed them a third time with the
// verbs. Three lists of one committee are three things to keep in step, and
// the page had already drifted — the rail's own note claimed it held the seats
// so the map would not, while the map held them anyway.
//
// The table wins because it is the only one of the three that can ADD and
// REMOVE a seat, and a reader who has just added someone should see them
// appear in the list they were looking at. The map keeps the picture it alone
// can draw, and engagement — the fact the rail contributed — rides the table as
// a column, where it sits beside the contact it describes.

import type { components } from "../../api/schema";
import { useCanWrite } from "../../app/capability";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { RelationshipRows } from "../relationshiprows";
import { AddRelationshipAction } from "../relationships";
import { CommitteeReading } from "./dealcommittee";

type DealCoverage = components["schemas"]["DealCoverage"];

/**
 * DealCommitteeCard is the deal's committee card: the coverage picture, then the
 * stakeholders themselves with the verbs that change them.
 */
export function DealCommitteeCard({
  dealId,
  coverage,
  withheld,
  pending,
  refusedReasonId,
}: Readonly<{
  dealId: string;
  coverage?: DealCoverage;
  withheld: boolean;
  pending: boolean;
  // The page's one sentence about why this deal takes no changes: a seat is
  // written through the deal's own write gate, so these verbs are refused by
  // the same fact as Edit.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const scope = { deal_id: dealId };
  const canCreate = useCanWrite("relationship", "create");
  // Engagement by contact, for the column below. The two reads are the same
  // edges — coverage counts them, /relationships lists them — so a row without
  // a seat is a row the coverage read did not cover (withheld, or still in
  // flight), and it says nothing rather than guessing "quiet", which would
  // report a silence nobody measured.
  const engagement = new Map(
    (coverage?.stakeholders ?? []).map((seat) => [
      seat.contact_id,
      seat.engaged,
    ]),
  );
  return (
    <Panel
      title={t("deal.committee.title")}
      titleAction={
        canCreate ? (
          <AddRelationshipAction
            scope={scope}
            refusedReasonId={refusedReasonId}
          />
        ) : undefined
      }
    >
      {/* The picture sits in the body's padding; the seats are ruled rows that
          run edge to edge under it, which is why the two are siblings here
          rather than one slot trying to be both. */}
      <PanelBody>
        <CommitteeReading
          coverage={coverage}
          withheld={withheld}
          pending={pending}
        />
      </PanelBody>
      <RelationshipRows
        scope={scope}
        refusedReasonId={refusedReasonId}
        stacked
        extra={{
          label: t("deal.committee.engagement"),
          render: (rel) => {
            const engaged = rel.contact_id
              ? engagement.get(rel.contact_id)
              : undefined;
            if (engaged === undefined) {
              return null;
            }
            return (
              <Badge tone={engaged ? "success" : undefined}>
                {engaged ? t("coverage.engaged") : t("coverage.quiet")}
              </Badge>
            );
          },
        }}
      />
    </Panel>
  );
}
