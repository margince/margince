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
import { Badge } from "../../design-system/atoms";
import type { ListChip } from "../../design-system/listsurface";
import { CellStrip } from "../../design-system/listtable";
import { useT } from "../../i18n";
import { isOption } from "../../lib/options";
import {
  KIND_LABELS,
  MEANING_LABELS,
  PARTNER_KINDS,
  PARTNER_STAGE_MEANINGS,
  partnerStageLabel,
  usePartnerStages,
} from "../../lib/partnerstages.queries";
import { unwrap } from "../../lib/problem";
import { CERT_LABELS, CERT_STATUSES, type Partner } from "./partners.vocab";

// #/partners (PartnersScreen) is the flat list, reached from the Companies
// list header. The Partner tab on a company is in partners.form.tsx.

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
  const kind = query.filters.partner_kind ?? query.filters.partner_role ?? "";
  const meaning = query.filters.stage_meaning ?? "";
  const cert = query.filters.cert_status ?? "";
  const data = unwrap(
    await api.GET("/partners", {
      params: {
        query: {
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          partner_kind: isOption(kind, PARTNER_KINDS) ? kind : undefined,
          relationship_stage: query.filters.relationship_stage || undefined,
          stage_meaning: isOption(meaning, PARTNER_STAGE_MEANINGS)
            ? meaning
            : undefined,
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
  // The stage names are the installation's own, so the dial carries words
  // rather than message keys.
  const stageChip: readonly ListChip[] = [
    {
      key: "relationship_stage",
      label: t("partner.stage"),
      allLabel: t("partner.stageAll"),
      options: (usePartnerStages().data ?? []).map((stage) => ({
        value: stage.key,
        label: stage.label,
      })),
    },
  ];

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
            key: "kinds",
            header: t("partner.kinds"),
            cell: (partner: Partner) =>
              partner.partner_kinds?.length ? (
                <CellStrip>
                  {PARTNER_KINDS.filter((kind) =>
                    partner.partner_kinds?.includes(kind),
                  ).map((kind) => (
                    <Badge key={kind}>{t(KIND_LABELS[kind])}</Badge>
                  ))}
                </CellStrip>
              ) : null,
          },
          {
            key: "cert",
            header: t("partner.certStatus"),
            cell: (partner: Partner) => t(CERT_LABELS[partner.cert_status]),
          },
          {
            key: "stage",
            header: t("partner.stage"),
            cell: (partner: Partner) => partnerStageLabel(partner, t),
          },
        ]}
        rowKey={(partner) => partner.company_id}
        rowRoute={(partner) => ({
          screen: "companies",
          id: partner.company_id,
        })}
        dataChips={stageChip}
        chips={[
          {
            key: "partner_kind",
            label: "partner.kinds",
            allLabel: "partner.kindsAll",
            options: PARTNER_KINDS.map((kind) => ({
              value: kind,
              label: KIND_LABELS[kind],
            })),
          },
          {
            key: "stage_meaning",
            label: "partner.stageMeaning",
            allLabel: "partner.stageMeaningAll",
            options: PARTNER_STAGE_MEANINGS.map((meaning) => ({
              value: meaning,
              label: MEANING_LABELS[meaning],
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
