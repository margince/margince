// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState, Skeleton } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import {
  type AccountIntroTarget,
  IntroRequestModal,
} from "./companycontacts/introrequest";
import { reachFunctionCount } from "./reachfunction";

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

// The reader's own row stays, because it shows at a glance they have a way in
// of their own. It reads "You" and leads, so nobody looks to ask themselves.
function readerFirst(
  colleagues: readonly Colleague[],
  viewerId: string | undefined,
): Colleague[] {
  const mine = colleagues.filter((colleague) => colleague.user_id === viewerId);
  const others = colleagues.filter(
    (colleague) => colleague.user_id !== viewerId,
  );
  return [...mine, ...others];
}

export function CompanyReachCard({
  companyId,
  companyName,
}: Readonly<{ companyId: string; companyName: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useCompanyReach(companyId);
  const viewerId = useViewerId();
  const colleagues = readerFirst(query.data?.colleagues ?? [], viewerId);
  const [asking, setAsking] = useState<AccountIntroTarget | null>(null);
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
            rows={colleagues}
            rowKey={(colleague) => colleague.user_id}
            columns={[
              {
                key: "colleague",
                header: t("companyReach.colleague"),
                grow: true,
                render: (colleague: Colleague) =>
                  colleague.user_id === viewerId
                    ? t("companyReach.you")
                    : colleague.display_name,
              },
              {
                key: "functions",
                header: t("companyReach.functions"),
                render: (colleague: Colleague) =>
                  colleague.functions
                    .map((held) => reachFunctionCount(t, locale, held))
                    .join(", "),
              },
              {
                key: "connections",
                header: t("companyReach.connections"),
                align: "end",
                render: (colleague: Colleague) =>
                  formatNumber(colleague.connections, locale),
              },
              {
                key: "ask",
                header: t("companyReach.ask"),
                headerHidden: true,
                align: "end",
                fold: "trail",
                // The reader's own row offers nothing: they need no
                // introduction to an account they already reach.
                render: (colleague: Colleague) =>
                  colleague.user_id === viewerId ? null : (
                    <Button
                      aria-label={t("companyReach.askNamed", {
                        name: colleague.display_name,
                      })}
                      onClick={() =>
                        setAsking({
                          viaUserId: colleague.user_id,
                          viaName: colleague.display_name,
                          companyName,
                          functions: colleague.functions,
                        })
                      }
                    >
                      {t("companyReach.ask")}
                    </Button>
                  ),
              },
            ]}
          />
        )}
        {/* Keyed on the colleague: the dialog holds a draft, and a second
            colleague's dialog must not open on the first one's message. */}
        {asking && (
          <IntroRequestModal
            key={asking.viaUserId}
            companyId={companyId}
            target={asking}
            onClose={() => setAsking(null)}
          />
        )}
      </PanelBody>
    </Panel>
  );
}
