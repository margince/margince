// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import { usePageName } from "../../app/pagemeta";
import { EntityRef } from "../../components/entityref";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "../../components/listquery";
import { useT } from "../../i18n";
import { isOption } from "../../lib/options";
import { unwrap } from "../../lib/problem";
import {
  CERT_LABELS,
  CERT_STATUSES,
  PARTNER_ROLES,
  type Partner,
  ROLE_LABELS,
  STAGE_LABELS,
} from "./partners.vocab";

// The Partner tab: a company IS a partner iff it has a `partner` row, so GET
// /companies/{id}/partner's 404 means "not a partner yet" rather than an
// error and renders an honest setup form instead of the shared error state.
// Both the setup and edit paths PUT the same UpsertPartnerRequest; first
// creation carries no If-Match (there is no prior version to precondition
// on), an edit carries the partner's own `version`. #/partners
// (PartnersScreen) is the flat list, reached from the Companies list header.

export { PartnerTab } from "./partners.form";

/**
 * One page of partners.
 *
 * No `sort`: `/partners` is keyset-paged by company id and orders by it,
 * so the ordering is not a dial anybody can turn. The parameter exists on the
 * operation and the handler never reads it, which is worse than its absence —
 * the list used to open on a "Newest" tab, send `sort=-created_at` and draw
 * rows in uuid order, telling the reader an ordering it did not have. Until the
 * store can answer one, this list offers none.
 */
async function fetchPartnersPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Partner>> {
  const role = query.filters.partner_role ?? "";
  const cert = query.filters.cert_status ?? "";
  const data = unwrap(
    await api.GET("/partners", {
      params: {
        query: {
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          partner_role: isOption(role, PARTNER_ROLES) ? role : undefined,
          cert_status: isOption(cert, CERT_STATUSES) ? cert : undefined,
        },
      },
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
    },
  };
}

export function PartnersScreen() {
  const t = useT();
  const pageName = usePageName("partners");
  const state = useListQuery<Partner>({
    key: "partners",
    fetchPage: fetchPartnersPage,
  });

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={state}
        unit="unit.partners"
        searchable={false}
        showArchivedToggle={false}
        columns={[
          {
            key: "company",
            header: t("partner.company"),
            // The Partner payload carries only company_id; EntityRef
            // hydrates the company name off the company read.
            // Named, not linked: the row's own identity link already goes to
            // this company, and a control inside that link would be invalid
            // markup offering the same destination twice.
            cell: (partner: Partner) => (
              <EntityRef kind="company" id={partner.company_id} asText />
            ),
            fixed: true,
          },
          {
            key: "role",
            header: t("partner.role"),
            cell: (partner: Partner) =>
              partner.partner_role ? t(ROLE_LABELS[partner.partner_role]) : "",
          },
          {
            key: "cert",
            header: t("partner.certStatus"),
            cell: (partner: Partner) => t(CERT_LABELS[partner.cert_status]),
          },
          {
            key: "stage",
            header: t("partner.stage"),
            cell: (partner: Partner) =>
              t(STAGE_LABELS[partner.relationship_stage]),
          },
        ]}
        rowKey={(partner) => partner.company_id}
        rowRoute={(partner) => ({
          screen: "companies",
          id: partner.company_id,
        })}
        chips={[
          {
            key: "partner_role",
            label: "partner.role",
            allLabel: "partner.roleAll",
            options: PARTNER_ROLES.map((role) => ({
              value: role,
              label: ROLE_LABELS[role],
            })),
          },
          {
            key: "cert_status",
            label: "partner.certStatus",
            allLabel: "partner.certStatusAll",
            options: CERT_STATUSES.map((status) => ({
              value: status,
              label: CERT_LABELS[status],
            })),
          },
        ]}
      />
    </div>
  );
}
