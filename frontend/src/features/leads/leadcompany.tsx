// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company a lead names, drawn one way wherever the lead page shows it. A
// linked company is a link. A typed name the server matched to one company
// carries an offer to link it. Any other typed name stands alone.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { leadWriteKeys } from "../../lib/leadkeys";
import { problemMessageOf, unwrap } from "../../lib/problem";

type Lead = components["schemas"]["Lead"];

// The offer to link the one company the lead's name matched, with the dialog
// that says both things the link does before it writes.
function LinkCompanyOffer({
  lead,
  match,
}: Readonly<{
  lead: Lead;
  match: components["schemas"]["LeadCompanyMatch"];
}>): ReactNode {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const link = useMutation({
    mutationKey: ["lead-link-company", lead.id],
    mutationFn: async (companyId: string) =>
      unwrap(
        await api.POST("/leads/{id}/link-company", {
          params: { path: { id: lead.id } },
          body: { company_id: companyId },
        }),
        t,
      ),
    onSuccess: () => {
      for (const key of leadWriteKeys(lead.id)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      setOpen(false);
    },
  });
  const close = () => {
    setOpen(false);
    link.reset();
  };
  return (
    <>
      <Button variant="link" onClick={() => setOpen(true)}>
        {t("lead.company.linkOffer", { name: match.name })}
      </Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("lead.company.linkTitle", { name: match.name })}
        confirmLabel={t("lead.company.linkConfirm")}
        onConfirm={() => link.mutate(match.id)}
        pending={link.isPending}
        error={link.isError ? problemMessageOf(link.error, t) : undefined}
      >
        <p className="t-body">
          {t("lead.company.linkOpens", { name: match.name })}
        </p>
        <p className="t-body">
          {t("lead.company.linkContact", { name: match.name })}
        </p>
      </ConfirmModal>
    </>
  );
}

// Every grant the link's writes ask for: the lead, the contact it lands on,
// and the employment edge between that contact and the company.
function useMayLinkCompany(): boolean {
  const lead = useCanWrite("lead", "update");
  const contactCreate = useCanWrite("contact", "create");
  const contactUpdate = useCanWrite("contact", "update");
  const relationship = useCanWrite("relationship", "create");
  return lead && contactCreate && contactUpdate && relationship;
}

// The company a lead names. `nameShown` is for a place that draws the typed
// name itself, such as an editable field. It then adds only the offer.
export function LeadCompany({
  lead,
  nameShown = false,
}: Readonly<{ lead: Lead; nameShown?: boolean }>): ReactNode {
  const name = lead.company_name?.trim();
  const mayLink = useMayLinkCompany();
  // The link reads the company's own name, which a rename there may have moved
  // away from what the lead typed.
  if (lead.company_id) {
    return nameShown ? null : <EntityRef kind="company" id={lead.company_id} />;
  }
  // A reader who may not change the lead cannot link it, so they get no offer.
  const match =
    lead.company_match && lead.writable && !lead.archived_at && mayLink
      ? lead.company_match
      : undefined;
  if (nameShown) {
    return match ? <LinkCompanyOffer lead={lead} match={match} /> : null;
  }
  if (!name) {
    return null;
  }
  return (
    <>
      <span>{name}</span>
      {match && <LinkCompanyOffer lead={lead} match={match} />}
    </>
  );
}
