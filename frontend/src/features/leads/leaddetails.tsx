// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { RecordListsActions } from "../../components/addtoshortlist";
import { ConversationSuggestionsCard } from "../../components/conversationsuggestions";
import { EntityRef } from "../../components/entityref";
import { useEntityName } from "../../components/entityref.queries";
import { LinkedInChip } from "../../components/linkedinchip";
import { type SectionLock, useSectionEdit } from "../../components/sectionedit";
import { LeadStops } from "../../components/stops";
import { TagsPanel } from "../../components/tagspanel";
import { Badge, PendingBody } from "../../design-system/atoms";
import { ContactLink } from "../../design-system/contactlink";
import { InfoTip } from "../../design-system/infotip";
import {
  type DetailRow,
  type DetailSection,
  RecordDetails,
} from "../../design-system/recorddetails";
import { formatDateAbbrev } from "../../format/format";
import { industryWords } from "../../format/industry";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  addedViaFor,
  originLabelFor,
  useLeadOrigins,
} from "../../lib/leadorigins";
import { useListsAvailable, useRecordLists } from "../../lib/lists.queries";
import { useRecordTags } from "../../lib/tags.queries";
import { useViewerZone } from "../../lib/viewerzone";
import { LeadCompany, useLeadCompanyFactsSection } from "./leadcompany";
import { useLeadAboutFlows } from "./leaddetailsflows";
import { useLeadEdit } from "./leadedit";
import { LeadOwnerReading, statusSetBy, useLeadOwnerFlow } from "./leadheader";
import { leadIndustry } from "./leadindustry";
import {
  LeadPartnerName,
  useLeadPartnerOptions,
  useLeadPartnerRow,
} from "./leadpartner";
import { LeadProductName } from "./leadproduct";
import { EmailValue, isBlank, reachRows } from "./leadreach";
import type { Lead, LeadWriter } from "./leads";

type TextKey =
  | "full_name"
  | "title"
  | "company_name"
  | "candidate_company_key"
  | "origin_detail";

// A linked lead's name is the company's own, so it is changed there, not here.
function useLinkedCompanyRow(lead: Lead): DetailRow | null {
  const t = useT();
  const linked = useEntityName("company", lead.company_id);
  if (!lead.company_id) return null;
  const settling =
    linked.name === null && (linked.reading === "pending" || linked.fetching);
  return {
    key: "company_name",
    term: t("create.companyName"),
    empty: !settling && !linked.name,
    value: settling ? (
      <PendingBody label={t("common.loading")} lines={1} />
    ) : (
      (linked.name ?? t("field.unset"))
    ),
  };
}

/** The About rows in the order the design reads them, blanks folded. */
function useAboutRows(lead: Lead, mayChange: boolean): DetailRow[] {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const origins = useLeadOrigins();
  const tags = useRecordTags("lead", lead.id, !lead.archived_at);
  const partnerRow = useLeadPartnerRow(lead);
  const text = (key: TextKey, label: MessageKey): DetailRow => ({
    key,
    term: t(label),
    empty: isBlank(lead[key]),
    value: isBlank(lead[key]) ? t("field.unset") : lead[key],
  });
  const companyName =
    useLinkedCompanyRow(lead) ?? text("company_name", "create.companyName");
  const tagged = (tags.data?.data ?? []).length > 0;
  return [
    {
      ...companyName,
      // A domain can match a company the lead never named, and the offer must show.
      empty: companyName.empty && !(lead.company_match && mayChange),
      value: (
        <>
          {companyName.value}
          <LeadCompany lead={lead} nameShown />
        </>
      ),
    },
    text("title", "create.contactTitle"),
    // The linked company's industry speaks for the lead, and shows under it.
    ...(leadIndustry(lead)?.from === "company"
      ? []
      : [
          {
            key: "industry",
            term: t("create.industry"),
            empty: !lead.industry,
            value: lead.industry
              ? industryWords(t, lead.industry)
              : t("field.unset"),
          },
        ]),
    ...reachRows(lead, t, {
      email: <EmailValue lead={lead} />,
      // The number dials, as on a contact.
      phone: lead.phone && (
        <ContactLink
          kind="phone"
          value={lead.phone}
          record={{ entityType: "lead", entityId: lead.id }}
          className="link-button lead-row-link"
          textClassName="lead-row-link"
        />
      ),
    }),
    {
      key: "origin",
      term: t("lead.origin"),
      empty: !lead.origin,
      value: lead.origin
        ? (originLabelFor(lead, origins.data) ?? lead.origin)
        : t("field.unset"),
    },
    text("origin_detail", "lead.originDetail"),
    ...(partnerRow ? [partnerRow] : []),
    {
      key: "added_via",
      term: t("lead.addedVia"),
      value: addedViaFor(lead, t),
    },
    {
      key: "received",
      term: t("lead.aside.received"),
      value: formatDateAbbrev(lead.created_at, locale, readerZone),
    },
    // The name heads the card; the row is only here to fill a blank one.
    ...(isBlank(lead.full_name) ? [text("full_name", "create.fullName")] : []),
    {
      key: "linkedin_url",
      term: t("create.linkedinUrl"),
      empty: isBlank(lead.linkedin_url),
      value: lead.linkedin_url ? (
        <LinkedInChip href={lead.linkedin_url} name={lead.full_name} />
      ) : (
        <span title={t("record.leadProfileReadOnly")}>{t("field.unset")}</span>
      ),
    },
    {
      ...text("candidate_company_key", "record.companyRoutingKey"),
      term: (
        <>
          {t("record.companyRoutingKey")}
          <InfoTip
            label={t("record.companyRoutingKeyWhy")}
            text={t("record.companyRoutingKeyHelp")}
            mark="help"
          />
        </>
      ),
    },
    {
      key: "project_id",
      term: t("lead.project"),
      empty: !lead.project_id,
      value: lead.project_id ? (
        <EntityRef kind="project" id={lead.project_id} />
      ) : (
        t("field.unset")
      ),
    },
    {
      key: "product_id",
      term: t("lead.product"),
      empty: !lead.product_id,
      value: <LeadProductName lead={lead} />,
    },
    {
      key: "tags",
      term: t("tags.panelTitle"),
      block: tagged,
      empty: !tagged,
      value: tagged ? (
        <TagsPanel
          entityType="lead"
          entityID={lead.id}
          bare
          archived={Boolean(lead.archived_at)}
          canEdit={false}
        />
      ) : (
        t("field.unset")
      ),
    },
  ];
}

// The reach card holds the address and the number, so it offers both.
function aboutKeys(rows: readonly DetailRow[]): string[] {
  const keys = rows.flatMap((row) =>
    row.key === "reach" ? ["email", "phone"] : [row.key],
  );
  return [...new Set(keys)];
}

/** What the About dialog shows of a field without offering it, and why. */
function useAboutLocks(lead: Lead): Record<string, SectionLock> {
  const t = useT();
  const partners = useLeadPartnerOptions();
  return {
    linkedin_url: { reason: t("record.leadProfileReadOnly") },
    ...(lead.company_id
      ? {
          company_name: {
            reason: t("lead.company.nameFollowsLink"),
            reading: <EntityRef kind="company" id={lead.company_id} asText />,
          },
        }
      : {}),
    ...(lead.partner_company_id && lead.partner_accepted_at
      ? {
          partner_company_id: {
            reason: t("lead.partnerAcceptedKept"),
            reading: (
              <LeadPartnerName
                partners={partners}
                id={lead.partner_company_id}
              />
            ),
          },
        }
      : {}),
  };
}

type DetailsProps = Readonly<{
  lead: Lead;
  writer: LeadWriter;
  terminalReasonId: string;
}>;

/**
 * The lead's details under its head: About, Ownership and Lists as flat
 * groups, each changed through its section's Edit. Every blank folds behind
 * one link, and a line says what a lead is. The deal and project cards live
 * on the Deals tab.
 */
export function LeadDetails(props: DetailsProps) {
  // The server answers no lists for an archived record. So the read sits
  // in a component that only a lead it can answer for mounts.
  return useListsAvailable() && !props.lead.archived_at ? (
    <ListedLeadDetails {...props} />
  ) : (
    <LeadDetailsBody {...props} />
  );
}

function ListedLeadDetails(props: DetailsProps) {
  const found = useRecordLists("lead", props.lead.id).data?.data ?? [];
  return (
    <LeadDetailsBody
      {...props}
      lists={
        <>
          {found.length > 0 && (
            <ul className="lead-aside-chips">
              {found.map((list) => (
                <li key={list.id}>
                  <Badge>{list.name}</Badge>
                </li>
              ))}
            </ul>
          )}
          <RecordListsActions entityType="lead" entityId={props.lead.id} />
        </>
      }
    />
  );
}

function LeadDetailsBody({
  lead,
  writer,
  terminalReasonId,
  lists,
}: DetailsProps & Readonly<{ lists?: ReactNode }>) {
  const t = useT();
  const about = useAboutRows(lead, !writer.readOnly);
  const edit = useLeadEdit(lead, writer);
  const aboutFlows = useLeadAboutFlows(lead, writer);
  const locks = useAboutLocks(lead);
  const aboutLabel = t("record.details.about");
  const aboutEdit = useSectionEdit(edit, {
    label: aboutLabel,
    keys: aboutKeys(about),
    flows: aboutFlows.flows,
    readOnly: locks,
  });
  const owner = useLeadOwnerFlow(lead, writer);
  const ownershipLabel = t("record.details.ownership");
  const ownershipEdit = useSectionEdit(edit, {
    label: ownershipLabel,
    keys: ["owner"],
    flows: owner.flow && { owner: owner.flow },
  });
  const setBy = statusSetBy(lead, t);
  const companyFacts = useLeadCompanyFactsSection(lead);
  const sections: DetailSection[] = [
    {
      key: "about",
      label: aboutLabel,
      edit: aboutEdit.edit,
      // A lead nobody may change has no blank to fill, so it has none to show.
      rows: writer.readOnly ? about.filter((row) => !row.empty) : about,
    },
    ...(companyFacts ? [companyFacts] : []),
    {
      key: "ownership",
      label: ownershipLabel,
      edit: ownershipEdit.edit,
      rows: [
        {
          key: "owner",
          term: t("lead.ownerLabel"),
          value: (
            <LeadOwnerReading
              lead={lead}
              writer={writer}
              terminalReasonId={terminalReasonId}
            />
          ),
        },
        {
          key: "status",
          term: t("lead.status"),
          // The row never wraps, so a narrow column cuts it; the title keeps
          // who or what set the status readable.
          value: <span title={setBy}>{setBy}</span>,
        },
      ],
    },
    { key: "lists", label: t("lists.record.title"), children: lists },
  ];
  return (
    <>
      <RecordDetails sections={sections} footnote={t("lead.aside.footnote")} />
      {aboutEdit.element}
      {aboutFlows.element}
      {ownershipEdit.element}
      {owner.element}
      <LeadStops leadId={lead.id} />
      {!lead.archived_at && (
        <ConversationSuggestionsCard
          key={lead.id}
          record={{ type: "lead", id: lead.id }}
        />
      )}
    </>
  );
}
