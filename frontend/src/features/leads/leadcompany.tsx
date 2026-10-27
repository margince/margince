// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company a lead names, drawn one way on the lead page, the lead table and
// the board. A linked company is a link. On the lead page alone, a domain or
// name the server matched to a company carries an offer to link it.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { formatSizeBand } from "../../components/companylookups";
import { EntityRef, useEntityName } from "../../components/entityref";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import type {
  DetailRow,
  DetailSection,
} from "../../design-system/recorddetails";
import { industryWords } from "../../format/industry";
import { jurisdictionName } from "../../format/jurisdiction";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { leadWriteKeys } from "../../lib/leadkeys";
import { problemMessageOf, unwrap } from "../../lib/problem";

type Lead = components["schemas"]["Lead"];

// The offer to link the company the lead's domain or name matched, with the rule
// that matched it. Its dialog says what the link does, and that the contact
// waits for qualifying.
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
  const reasonId = useId();
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
      <Button
        variant="link"
        className="btn-valuelabel lead-company-offer"
        aria-describedby={reasonId}
        onClick={() => setOpen(true)}
      >
        {t("lead.company.linkOffer", { name: match.name })}
      </Button>{" "}
      <span id={reasonId} className="t-caption">
        {match.matched_by === "domain"
          ? t("lead.company.matchedByDomain")
          : t("lead.company.matchedByName")}
      </span>
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
          {match.matched_by === "domain"
            ? t("lead.company.linkWhyDomain", { name: match.name })
            : t("lead.company.linkWhyName", { name: match.name })}{" "}
          {t("lead.company.linkOpens", { name: match.name })}
        </p>
        <p className="t-body">
          {t("lead.company.linkOnQualify", { name: match.name })}
        </p>
      </ConfirmModal>
    </>
  );
}

// The company a lead names, without the offer: what a list row or a board card
// draws. The link reads the company's own name, which a rename there may have
// moved away from what the lead typed.
export function LeadCompanyName({ lead }: Readonly<{ lead: Lead }>): ReactNode {
  if (lead.company_id) {
    return <EntityRef kind="company" id={lead.company_id} />;
  }
  const name = lead.company_name?.trim();
  return name ? <span>{name}</span> : null;
}

// The name a figure prints for the lead's company. A linked company's own name
// comes from the cache entry its link reads, and the typed name stands in until then.
export function useLeadCompanyName(lead: Lead): string | null {
  const linked = useEntityName("company", lead.company_id);
  return linked.name ?? (lead.company_name?.trim() || null);
}

// The company a lead names. `nameShown` is for a place that draws the typed
// name itself, such as an editable field. It then adds only the offer.
export function LeadCompany({
  lead,
  nameShown = false,
}: Readonly<{ lead: Lead; nameShown?: boolean }>): ReactNode {
  const mayLink = useCanWrite("lead", "update");
  if (lead.company_id) {
    return nameShown ? null : <LeadCompanyName lead={lead} />;
  }
  // A reader who may not change the lead cannot link it, so they get no offer.
  const match =
    lead.company_match && lead.writable && !lead.archived_at && mayLink
      ? lead.company_match
      : undefined;
  if (nameShown || !lead.company_name?.trim()) {
    return match ? <LinkCompanyOffer lead={lead} match={match} /> : null;
  }
  return (
    <>
      <LeadCompanyName lead={lead} />
      {match && <LinkCompanyOffer lead={lead} match={match} />}
    </>
  );
}

// The facts are the company record's and never written through the lead, so
// the group is named for the company and says where they change.
export function useLeadCompanyFactsSection(
  lead: Lead,
): DetailSection | undefined {
  const t = useT();
  const { locale } = useLocale();
  const name = useLeadCompanyName(lead);
  const facts = lead.company_facts;
  const rows: DetailRow[] = [];
  if (facts?.industry) {
    rows.push({
      key: "company_industry",
      term: t("create.industry"),
      value: industryWords(t, facts.industry),
    });
  }
  if (facts?.country) {
    rows.push({
      key: "company_country",
      term: t("lead.details.companyCountry"),
      value: jurisdictionName(facts.country, locale),
    });
  }
  if (facts?.size_band) {
    rows.push({
      key: "company_size",
      term: t("lead.companySize"),
      value: formatSizeBand(t, facts.size_band),
    });
  }
  if (rows.length === 0) {
    return undefined;
  }
  return {
    key: "company",
    label: name
      ? t("lead.details.fromCompany", { name })
      : t("lead.details.fromCompanyRecord"),
    rows,
    children: <p className="t-caption">{t("lead.details.companyDecides")}</p>,
  };
}
