// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { useRecordAssignments } from "../../../components/recordassignments.queries";
import { RelationshipRows } from "../../../components/relationshiprows";
import { AddRelationshipAction } from "../../../components/relationships";
import { Disclosure } from "../../../design-system/disclosure";
import { OverviewCard } from "../../../design-system/overviewcard";
import { activityDay } from "../../../format/activitywhen";
import { useLocale, useT } from "../../../i18n";
import { useCanWrite } from "../../../lib/capability";
import { useMemberName } from "../../../lib/membernames";
import { useViewerZone } from "../../../lib/viewerzone";
import { type AddSeat, CommitteeReading, committeeFoot } from "./dealcommittee";
import { CommitteeLines } from "./dealcommitteelinelist";
import { committeeLines } from "./dealcommitteelines";
import { committeeTeam } from "./dealcommitteeteam";

type DealCoverage = components["schemas"]["DealCoverage"];

/**
 * DealCommitteeCard is the deal's committee on its Overview. It names who of
 * ours works the deal and in what job, and who of the buyer sits on it and how
 * engaged. Each seat the coverage rules find missing carries the verb that
 * fills it, and each buyer seat opens its contact.
 *
 * A reader who may change or remove a stakeholder finds each edge under a
 * fold below the committee, with the same Edit and Remove the relationship
 * panel offers everywhere else. A seat stays a link to its contact.
 */
export function DealCommitteeCard({
  dealId,
  ownerId,
  coverage,
  withheld,
  pending,
  refusedReasonId,
  canWrite,
  roleLabel,
}: Readonly<{
  dealId: string;
  ownerId?: string | null;
  coverage?: DealCoverage;
  withheld: boolean;
  pending: boolean;
  // The page's one sentence about why this deal takes no changes. A seat is
  // written through the deal's own write gate, so the same fact that refuses
  // Edit refuses the verb.
  refusedReasonId?: string;
  // Whether this reader may write the deal. A seat is written through the
  // deal's write gate, so a reader it refuses is offered no seat verb.
  canWrite: boolean;
  // A stakeholder role in the reader's words.
  roleLabel: (role: string) => string;
}>) {
  const t = useT();
  // Both asked on every render: hooks run in one order, so no short circuit.
  const canCreate = useCanWrite("relationship", "create") && canWrite;
  const canUpdate = useCanWrite("relationship", "update");
  const canDelete = useCanWrite("relationship", "delete");
  const canChange = (canUpdate || canDelete) && canWrite;
  const { locale } = useLocale();
  const zone = useViewerZone();
  const assignments = useRecordAssignments("deal", dealId);
  const ownerName =
    useMemberName(ownerId).data ??
    assignments.data?.find((held) => held.subject_id === ownerId)?.subject_name;
  const team = committeeTeam({
    owner: ownerId && ownerName ? { id: ownerId, name: ownerName } : undefined,
    assignments: assignments.data ?? [],
    coverage,
    t,
    locale,
  });
  const drawn = committeeLines(coverage, t, (at, precision) =>
    activityDay(at, precision, locale, zone),
  );
  const foot = committeeFoot(coverage, withheld, t, locale);
  const addSeat: AddSeat | undefined = canCreate
    ? (gap, describedBy) => (
        <AddRelationshipAction
          scope={{ deal_id: dealId }}
          refusedReasonId={refusedReasonId}
          variant="link"
          seatRole={gap.role}
          describedBy={describedBy}
        />
      )
    : undefined;
  return (
    <OverviewCard
      title={t("deal.committee.card")}
      action={
        canCreate ? (
          <AddRelationshipAction
            scope={{ deal_id: dealId }}
            refusedReasonId={refusedReasonId}
            variant="link"
          />
        ) : undefined
      }
      foot={foot?.text}
      footTone={foot?.warn ? "warn" : undefined}
    >
      <CommitteeReading
        coverage={coverage}
        withheld={withheld}
        pending={pending}
        team={team}
        teamPending={assignments.isPending}
        teamUnread={assignments.isError}
        roleLabel={roleLabel}
        wroteOn={(at, precision) => activityDay(at, precision, locale, zone)}
        addSeat={addSeat}
      />
      {drawn && (
        <CommitteeLines lines={drawn.lines} unreached={drawn.unreached} />
      )}
      {canChange && !withheld && (
        <Disclosure summary={t("deal.committee.change")}>
          <RelationshipRows
            scope={{ deal_id: dealId }}
            refusedReasonId={refusedReasonId}
            stacked
          />
        </Disclosure>
      )}
    </OverviewCard>
  );
}
