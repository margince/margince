// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { OverviewCard } from "../../../design-system/overviewcard";
import { useLocale, useT } from "../../../i18n";
import { useCanWrite } from "../../../lib/capability";
import { useMemberName } from "../../../lib/membernames";
import { AddRelationshipAction } from "../../records/relationships";
import { CommitteeReading, committeeFoot } from "./dealcommittee";
import { committeeOf } from "./dealstanding";

type DealCoverage = components["schemas"]["DealCoverage"];

/**
 * DealCommitteeCard is the deal's committee on its Overview. It shows who of the
 * buyer is on it, who we are in touch with, and the seats the coverage rules
 * find missing. Each seat opens its contact.
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
  const owner = useMemberName(ownerId).data ?? undefined;
  const foot = committeeFoot(coverage, withheld, t, locale);
  // Nobody named and no seat missing is an answer, drawn as every card's
  // empty row; a withheld or pending read keeps the picture's own states.
  const committee = committeeOf(coverage, withheld);
  const nobody =
    !pending &&
    committee.state === "read" &&
    committee.named === 0 &&
    committee.gaps === 0;
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
      empty={t("deal.standing.committee.none")}
      foot={foot?.text}
      footTone={foot?.warn ? "warn" : undefined}
    >
      {!nobody && (
        <CommitteeReading
          coverage={coverage}
          withheld={withheld}
          pending={pending}
          owner={owner}
          roleLabel={roleLabel}
        />
      )}
    </OverviewCard>
  );
}
