// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { InlineChoice } from "../../design-system/inlinechoice";
import type { DetailRow } from "../../design-system/recorddetails";
import { formatDateAbbrev } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { leadWriteKeys } from "../../lib/leadkeys";
import { usePartnerCompanyOptions } from "../../lib/partnerstages.queries";
import { unwrap } from "../../lib/problem";
import type { Lead, LeadWriter } from "./leads";

// Accepting a partner's lead activates the partner. So the verb waits for a
// partner on the lead and for a seat that may change partners.
function AcceptForPartner({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const accept = useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/leads/{id}/partner-acceptance", {
          params: { path: { id } },
        }),
      ),
    onSuccess: async () => {
      for (const queryKey of [
        ...leadWriteKeys(lead.id),
        ["partner"],
        ["partners"],
      ]) {
        await queryClient.invalidateQueries({ queryKey });
      }
    },
  });
  return (
    <>
      <Button pending={accept.isPending} onClick={() => accept.mutate(lead.id)}>
        {t("lead.partnerAccept")}
      </Button>
      <ErrorLine error={accept.error} />
    </>
  );
}

/**
 * The "Referred by partner" row, drawn once the installation has a partner or
 * the lead already names one. A lead accepted for its partner keeps it.
 */
export function useLeadPartnerRow(
  lead: Lead,
  writer: Pick<LeadWriter, "readOnly" | "readOnlyReason">,
  onSave: (next: string) => Promise<void>,
): DetailRow | null {
  const t = useT();
  const { locale } = useLocale();
  const managesPartners = useCan("partner", "update");
  const options = usePartnerCompanyOptions(useCan("company", "read")).data;
  if ((options ?? []).length === 0 && !lead.partner_company_id) {
    return null;
  }
  const accepted = lead.partner_accepted_at;
  const name = (id: string) =>
    options?.find((option) => option.value === id)?.label ?? id;
  return {
    key: "partner_company_id",
    term: t("lead.partner"),
    empty: !lead.partner_company_id,
    value: (
      <>
        <InlineChoice
          label={t("lead.partner")}
          hideLabel
          value={lead.partner_company_id ?? ""}
          options={[
            { value: "", label: t("lead.partnerNone") },
            ...(options ?? []),
          ]}
          render={(value) => (value === "" ? t("field.unset") : name(value))}
          canEdit={!writer.readOnly && !accepted}
          readOnlyReason={
            accepted ? t("lead.partnerAcceptedKept") : writer.readOnlyReason
          }
          onSave={onSave}
        />
        {accepted ? (
          <span className="t-caption">
            {t("lead.partnerAccepted", {
              date: formatDateAbbrev(accepted, locale, viewerZone()),
            })}
          </span>
        ) : (
          lead.partner_company_id &&
          managesPartners &&
          !lead.archived_at && <AcceptForPartner lead={lead} />
        )}
      </>
    ),
  };
}
