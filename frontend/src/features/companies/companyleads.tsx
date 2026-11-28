// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { AppLink } from "../../design-system/applink";
import { RecordDetails } from "../../design-system/recorddetails";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { leadIdentityName } from "../../lib/leadname";
import { routeHash } from "../../lib/router";

type Company360 = components["schemas"]["Company360"];

export function CompanyLeads({
  companyId,
  view,
  loading,
}: Readonly<{ companyId: string; view?: Company360; loading: boolean }>) {
  const t = useT();
  const leads = view?.leads;
  const state = sectionState(
    view,
    "leads",
    leads !== undefined,
    leads?.data.length ?? 0,
    loading,
  );
  const href = `${routeHash({ screen: "leads" })}?${new URLSearchParams({ company_id: companyId }).toString()}`;
  const children = (
    <>
      <SurfaceState
        state={state === "ready" && leads?.page.has_more ? "partial" : state}
        emptyLabel={t("co.leads.empty")}
        loadingLabel={t("co.leads.title")}
      >
        <ul>
          {leads?.data.map((lead) => (
            <li key={lead.id}>
              <EntityRef
                kind="lead"
                id={lead.id}
                name={leadIdentityName(lead)}
                archived={Boolean(lead.archived_at)}
              />
            </li>
          ))}
        </ul>
      </SurfaceState>
      {state === "ready" && (
        <AppLink href={href}>
          {t(leads?.page.has_more ? "co.leads.more" : "co.leads.list")}
        </AppLink>
      )}
    </>
  );
  return (
    <RecordDetails
      sections={[{ key: "leads", label: t("co.leads.title"), children }]}
    />
  );
}
