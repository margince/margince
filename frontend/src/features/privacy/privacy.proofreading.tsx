// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { FactList } from "../../design-system/factlist";
import { formatDate, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { HUMAN_ACTOR_PREFIX } from "../../lib/auditactor";
import { IDENTITY_CHECK_LABEL, IDENTITY_CHECKS } from "../../lib/privacy.logic";
import { isUuid } from "../../lib/uuid";
import { useViewerZone } from "../../lib/viewerzone";

export function RequestProof({
  proof,
}: Readonly<{ proof: components["schemas"]["DataSubjectRequestProof"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const identity = IDENTITY_CHECKS.find(
    (kind) => kind === proof.identity_check,
  );
  const completedBy = proof.completed_by_id?.startsWith(HUMAN_ACTOR_PREFIX)
    ? proof.completed_by_id.slice(HUMAN_ACTOR_PREFIX.length)
    : proof.completed_by_id;
  const unknown = t("privacy.proofUnknown");
  const count = Object.values(proof.erased_counts).reduce(
    (sum, value) => sum + value,
    0,
  );
  return (
    <FactList
      facts={[
        {
          key: "reference",
          term: t("privacy.proofReference"),
          value: proof.request_number,
        },
        {
          key: "received",
          term: t("privacy.receivedAt"),
          value: formatDate(proof.received_at, locale, zone),
        },
        {
          key: "answered",
          term: t("privacy.proofAnswered"),
          value: formatDate(proof.answered_at, locale, zone),
        },
        {
          key: "kept",
          term: t("privacy.proofKeptUntil"),
          value: formatDate(proof.keep_until, locale, zone),
        },
        {
          key: "identity",
          term: t("privacy.identityCheck"),
          value: identity ? t(IDENTITY_CHECK_LABEL[identity]) : unknown,
        },
        {
          key: "requester",
          term: t("privacy.requesterEmail"),
          value: proof.requester_email ?? unknown,
        },
        {
          key: "closer",
          term: t("privacy.proofCompletedBy"),
          value:
            proof.completed_by_type === "human" &&
            completedBy &&
            isUuid(completedBy) ? (
              <EntityRef kind="user" id={completedBy} />
            ) : proof.completed_by_type === "human" ? (
              unknown
            ) : proof.completed_by_type ? (
              t("privacy.proofAutomated")
            ) : (
              unknown
            ),
        },
        {
          key: "counts",
          term: t("privacy.proofErasureChanges"),
          value: proof.erased_counts_known
            ? formatNumber(count, locale)
            : unknown,
          note:
            proof.erased_uncounted.length > 0
              ? t("privacy.proofPartialCounts")
              : t("privacy.proofExportCounts"),
        },
      ]}
    />
  );
}
