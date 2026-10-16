// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { EmptyState, Skeleton } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, unwrap } from "../lib/problem";

// Who on the team can get us in at this account.
//
// The sibling card in linkedin-reach.tsx answers this about the reader's own
// network. This one answers it about colleagues, which is the warm route a rep
// wants: somebody here is already connected at this company.
//
// A count and never a list of names. The connections are third parties who
// never consented to appearing in this CRM. A colleague's account stays theirs,
// because the number is a derived fact about reach and not access to it.

type Reach = components["schemas"]["CompanyLinkedInReachResponse"];
type Colleague = components["schemas"]["CompanyLinkedInReachColleague"];

function useCompanyReach(companyId: string) {
  return useQuery({
    queryKey: ["company-linkedin-reach", companyId],
    queryFn: async (): Promise<Reach> => {
      return unwrap(
        await api.GET("/companies/{id}/linkedin-reach", {
          params: { path: { id: companyId } },
        }),
      );
    },
  });
}

export function CompanyReachCard({ companyId }: { companyId: string }) {
  const t = useT();
  const { locale } = useLocale();
  const query = useCompanyReach(companyId);
  const colleagues = query.data?.colleagues ?? [];
  return (
    <Panel title={t("companyReach.title")}>
      <PanelBody>
        <PanelIntro>{t("companyReach.sub")}</PanelIntro>
        {query.isPending && <Skeleton width="60%" />}
        {/* A refused read is not an empty one. Drawn as "nobody reaches this
            account" it would read as a cold door rather than an answer this
            reader was not given. */}
        {query.isError && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("companyReach.readFailed")}
          >
            {problemMessageOf(query.error, t)}
          </Callout>
        )}
        {query.isSuccess && colleagues.length === 0 && (
          <EmptyState>{t("companyReach.none")}</EmptyState>
        )}
        {query.isSuccess && colleagues.length > 0 && (
          <DataTable
            bleed
            fold
            label={t("companyReach.colleaguesLabel")}
            rows={[...colleagues]}
            rowKey={(colleague) => colleague.user_id}
            columns={[
              {
                key: "colleague",
                header: t("companyReach.colleague"),
                grow: true,
                render: (colleague: Colleague) => colleague.display_name,
              },
              {
                key: "connections",
                header: t("companyReach.connections"),
                align: "end",
                render: (colleague: Colleague) =>
                  formatNumber(colleague.connections, locale),
              },
            ]}
          />
        )}
      </PanelBody>
    </Panel>
  );
}
