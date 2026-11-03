// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { useRecordAssignments } from "../../../components/recordassignments.queries";
import { OverviewCard } from "../../../design-system/overviewcard";
import { activityDay } from "../../../format/activitywhen";
import { useLocale, useT } from "../../../i18n";
import { useCanWrite } from "../../../lib/capability";
import { useMemberName } from "../../../lib/membernames";
import { useRecordZone } from "../../../lib/recordzone";
import { AddRelationshipAction } from "../../records/relationships";
import { type AddSeat, CommitteeReading, committeeFoot } from "./dealcommittee";
import { committeeTeam } from "./dealcommitteeteam";

type DealCoverage = components["schemas"]["DealCoverage"];

/**
 * DealCommitteeCard is the deal's committee on its Overview. It names who of
 * ours works the deal and in what job, and who of the buyer sits on it and how
 * engaged. Each seat the coverage rules find missing carries the verb that
 * fills it, and each buyer seat opens its contact.
 */
export function DealCommitteeCard({
  dealId,
  ownerId,
  coverage,
  withheld,
  pending,
  refusedReasonId,
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
  // A stakeholder role in the reader's words.
  roleLabel: (role: string) => string;
}>) {
  const t = useT();
  const canCreate = useCanWrite("relationship", "create");
  const { locale } = useLocale();
  const zone = useRecordZone();
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
    </OverviewCard>
  );
}
