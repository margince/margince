// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEmployerSummary } from "../../components/contactemployersummary";
import { EntityRef } from "../../components/entityref";
import { Badge } from "../../design-system/atoms";
import { RecordCard } from "../../design-system/recordcard";
import { LIFECYCLE_LABELS } from "../../format/companylifecycle";
import { industryWords } from "../../format/industry";
import { useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import { routeHash } from "../../lib/router";

// The account is the same card as a contact in Way in. Mark and name are one
// link, its facts sit under them and its stage at the edge.
export function DealAccount({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const company = useEmployerSummary(companyId).data;
  const name = company?.display_name;
  const stage =
    company?.status && company.status !== "unknown"
      ? t(labelFor(LIFECYCLE_LABELS, company.status))
      : undefined;
  const facts = [
    stage,
    company?.industry ? industryWords(t, company.industry) : undefined,
    company?.address?.city,
  ].filter((part): part is string => Boolean(part));
  if (!name) {
    return <EntityRef kind="company" id={companyId} />;
  }
  return (
    <RecordCard
      kind="company"
      name={name}
      identity={companyId}
      href={routeHash({ screen: "companies", id: companyId })}
      logo={company?.logo_url}
      position={facts.length > 0 ? facts.join(" · ") : undefined}
      aside={
        company?.archived_at ? (
          <Badge tone="warning">{t("record.archived")}</Badge>
        ) : (
          stage && <Badge wrap>{stage}</Badge>
        )
      }
    />
  );
}
