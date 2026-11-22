// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deals sellers asked the team's manager to look at, beside the ranked list.
// Each row says why the seller asked, in the words of the facts the move named.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import { Badge, Button } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatDate } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { AFTER_THE_DAY } from "./worklist.layout";
import { type ForecastFocus, worklistKey } from "./worklist.queries";

type Request = NonNullable<ForecastFocus["requests"]>[number];

const REASON_KEYS = {
  big_deal: "worklist.gap.reason.big_deal",
  single_contact: "worklist.gap.reason.single_contact",
  stuck: "worklist.gap.reason.stuck",
} as const;

export function RequestedDeals({
  requests,
}: Readonly<{ requests: Request[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const queryClient = useQueryClient();
  const handled = useMutation({
    mutationFn: async (dealId: string) =>
      unwrap(
        await api.DELETE("/deals/{id}/manager-request", {
          params: { path: { id: dealId } },
        }),
        t,
      ),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: worklistKey }),
  });
  if (requests.length === 0) return null;
  return (
    <Panel className={AFTER_THE_DAY} title={t("worklist.gap.requested.title")}>
      <PanelBody>
        <DataTable
          label={t("worklist.gap.requested.title")}
          rows={requests}
          rowKey={(request) => request.deal_id}
          columns={[
            {
              key: "deal",
              header: t("worklist.gap.deal"),
              render: (request: Request) => (
                <EntityRef
                  kind="deal"
                  id={request.deal_id}
                  name={request.deal_name}
                />
              ),
            },
            {
              key: "owner",
              header: t("worklist.gap.owner"),
              render: (request: Request) => (
                <EntityRef kind="user" id={request.owner_id} />
              ),
            },
            {
              key: "why",
              header: t("worklist.gap.why"),
              render: (request: Request) => (
                <>
                  {request.reasons.map((reason) => (
                    <Badge key={reason} tone="warning">
                      {reason in REASON_KEYS
                        ? t(REASON_KEYS[reason as keyof typeof REASON_KEYS])
                        : reason}
                    </Badge>
                  ))}
                  <span className="t-caption">
                    {formatDate(request.requested_at, locale, zone)}
                  </span>
                </>
              ),
            },
            {
              key: "done",
              header: "",
              render: (request: Request) => (
                <Button
                  pending={handled.isPending}
                  onClick={() => handled.mutate(request.deal_id)}
                >
                  {t("worklist.gap.requested.done")}
                </Button>
              ),
            },
          ]}
        />
        <ErrorLine error={handled.error} />
      </PanelBody>
    </Panel>
  );
}
