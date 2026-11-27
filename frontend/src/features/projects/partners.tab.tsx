// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EmptyState, SectionHeader } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useWriteRefusal } from "../../lib/capability";
import { throwProblem } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { PartnerCommissions } from "./partnercommissions";
import { PartnerDeals } from "./partnerdeals";
import { PartnerDetail, PartnerForm } from "./partners.form";
import type { Partner } from "./partners.vocab";

async function fetchPartner(companyId: string): Promise<Partner | null> {
  const { data, error, response } = await api.GET("/companies/{id}/partner", {
    params: { path: { id: companyId } },
  });
  if (response.status === 204) {
    return null;
  }
  if (error) {
    throwProblem(error);
  }
  return data ?? null;
}

// The Partner tab. A company is a partner when it has a `partner` row, so a 204
// from GET /companies/{id}/partner means "not a partner yet", not an error. It
// renders the setup form instead of the shared error state. Setup and edit
// both PUT the same UpsertPartnerRequest. The first creation carries no
// If-Match, since there is no prior version; an edit carries the partner's
// own `version`.
export function PartnerTab({
  companyId,
  programEnded,
}: Readonly<{ companyId: string; programEnded?: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();
  // Every partner write needs the company grant; the tier alone also needs
  // the partner grant, which the form asks for itself.
  const writeRefusal = useWriteRefusal("company", "update", {
    seat: t("settings.seatRefused"),
    grant: t("partner.writeRefused"),
  });
  const refusal = useMe().data?.authorization ? writeRefusal : undefined;
  const query = useQuery({
    queryKey: ["partner", companyId],
    queryFn: () => fetchPartner(companyId),
  });

  function invalidateAfterSave() {
    queryClient.invalidateQueries({ queryKey: ["partner", companyId] });
    queryClient.invalidateQueries({
      queryKey: ["company", companyId],
    });
    queryClient.invalidateQueries({ queryKey: ["companies"] });
    // The deal form asks this list whether a partner programme exists at all,
    // so making the first partner has to reach it. Otherwise its fields stay
    // absent there until the cache goes stale, and the setting looks lost.
    queryClient.invalidateQueries({ queryKey: ["partners"] });
  }

  return (
    <QueryGate query={query} pendingLabel={t("nav.partners")}>
      {(partner) => (
        <div className="record-stack">
          {partner ? (
            <PartnerDetail
              companyId={companyId}
              partner={partner}
              refusal={refusal}
              onSaved={invalidateAfterSave}
            />
          ) : (
            // Two bodies: the panel's own seam divides what there is none of
            // from how the first one is made.
            <Panel title={t("tab.partner")}>
              <PanelBody>
                <EmptyState>{t("partner.none")}</EmptyState>
              </PanelBody>
              <PanelBody>
                <SectionHeader title={t("partner.setup")} />
                {refusal ? (
                  <p>{refusal}</p>
                ) : (
                  <PartnerForm
                    companyId={companyId}
                    intent="create"
                    onSaved={invalidateAfterSave}
                  />
                )}
              </PanelBody>
            </Panel>
          )}
          {(partner || programEnded) && (
            <>
              <PartnerDeals companyId={companyId} />
              <PartnerCommissions companyId={companyId} />
            </>
          )}
        </div>
      )}
    </QueryGate>
  );
}
