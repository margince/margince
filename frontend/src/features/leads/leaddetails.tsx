// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { EntityRef } from "../../components/entityref";
import { saveRecordEdit } from "../../components/recordedit";
import { TagsPanel } from "../../components/tagspanel";
import { Badge } from "../../design-system/atoms";
import {
  ContactLink,
  CopyEmail,
  EmailFace,
} from "../../design-system/contactlink";
import { InlineChoice } from "../../design-system/inlinechoice";
import { InlineText } from "../../design-system/inlinetext";
import { OffsiteLink } from "../../design-system/offsitelink";
import {
  type DetailRow,
  type DetailSection,
  RecordDetails,
} from "../../design-system/recorddetails";
import { formatDateAbbrev } from "../../format/format";
import { industryOptions, industryWords } from "../../format/industry";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWriteRecord } from "../../lib/capability";
import { leadWriteKeys } from "../../lib/leadkeys";
import {
  addedViaFor,
  originLabelFor,
  originPickOptions,
  useLeadOrigins,
} from "../../lib/leadorigins";
import { useListsAvailable, useRecordLists } from "../../lib/lists.queries";
import { ProblemError } from "../../lib/problem";
import { useRecordTags } from "../../lib/tags.queries";
import { LeadCompany, useLeadCompanyFactsSection } from "./leadcompany";
import { LeadOwnerControl, statusSetBy } from "./leadheader";
import { LeadProjectAttachButton } from "./leadraillinks";
import type { Lead, LeadWriter } from "./leads";
import { mapLeadUpdate } from "./leads.form";

type TextKey =
  | "full_name"
  | "email"
  | "phone"
  | "title"
  | "company_name"
  | "candidate_company_key"
  | "origin_detail";

function isBlank(value: string | null | undefined): boolean {
  return !value?.trim();
}

/**
 * One field of the lead, edited where it stands. Every row saves through
 * the same independent-edit path the record form used, so two rows cannot
 * send a different If-Match or invalidate different caches.
 */
function useFieldSave(lead: Lead) {
  const t = useT();
  const queryClient = useQueryClient();
  return (key: TextKey | "origin" | "industry", required = false) =>
    async (next: string) => {
      if (required && !next.trim()) {
        throw new ProblemError({ detail: t("record.fieldRequired") });
      }
      await saveRecordEdit("lead", lead, mapLeadUpdate({ [key]: next }));
      for (const queryKey of leadWriteKeys(lead.id)) {
        await queryClient.invalidateQueries({ queryKey });
      }
    };
}

// The address as the door to writing to the lead. Its edit verb is drawn only
// while the row is pointed at or focused, so the row reads as one line.
function EmailValue({
  lead,
  canEdit,
  readOnlyReason,
  onSave,
}: Readonly<{
  lead: Lead;
  canEdit: boolean;
  readOnlyReason?: string;
  onSave: (next: string) => Promise<void>;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  // With no address there is nothing to follow, so the blank is the editor.
  if (!lead.email) {
    return (
      <InlineText
        label={t("create.email")}
        value=""
        type="email"
        placeholder={t("field.unset")}
        canEdit={canEdit}
        readOnlyReason={readOnlyReason}
        onSave={onSave}
      />
    );
  }
  return (
    <span className="lead-row-value">
      {!editing && (
        <>
          <ContactLink
            kind="email"
            value={lead.email}
            record={{ entityType: "lead", entityId: lead.id }}
            readOnly={Boolean(lead.archived_at)}
            className="link-button lead-row-link"
            textClassName="lead-row-link"
          >
            <EmailFace address={lead.email} />
          </ContactLink>
          <CopyEmail address={lead.email} />
        </>
      )}
      {canEdit && (
        <span className={editing ? undefined : "lead-row-verb"}>
          <InlineText
            label={t("create.email")}
            value={lead.email ?? ""}
            verb
            type="email"
            placeholder={t("field.unset")}
            canEdit={canEdit}
            readOnlyReason={readOnlyReason}
            onEditingChange={setEditing}
            onSave={onSave}
          />
        </span>
      )}
    </span>
  );
}

/** The About rows in the order the design reads them, blanks folded. */
function useAboutRows(
  lead: Lead,
  writer: LeadWriter,
  reasonId: string | undefined,
): DetailRow[] {
  const t = useT();
  const { locale } = useLocale();
  const origins = useLeadOrigins();
  const tags = useRecordTags("lead", lead.id, !lead.archived_at);
  const mayTag = useCanWriteRecord("lead", lead);
  const save = useFieldSave(lead);
  const canEdit = !writer.readOnly;
  const reason = writer.readOnlyReason;
  const text = (
    key: TextKey,
    label: MessageKey,
    options: { required?: boolean; type?: "email" } = {},
  ): DetailRow => ({
    key,
    term: t(label),
    empty: isBlank(lead[key]),
    value: (
      <InlineText
        label={t(label)}
        value={lead[key] ?? ""}
        type={options.type}
        placeholder={t("field.unset")}
        canEdit={canEdit}
        readOnlyReason={reason}
        onSave={save(key, options.required)}
      />
    ),
  });
  // "Not set" is a real choice: choosing it sends null and the lead scores neutral.
  const originOptions = [
    { value: "", label: t("lead.originNone") },
    ...originPickOptions(origins.data, lead.origin),
  ];
  const companyName = text("company_name", "create.companyName");
  const rows: DetailRow[] = [
    {
      ...companyName,
      // A domain can match a company the lead never named, and the offer must show.
      empty: companyName.empty && !(lead.company_match && canEdit),
      value: (
        <>
          {companyName.value}
          <LeadCompany lead={lead} nameShown />
        </>
      ),
    },
    text("title", "create.contactTitle"),
    // The linked company's industry speaks for the lead, and shows under it.
    ...(lead.company_facts?.industry
      ? []
      : [
          {
            key: "industry",
            term: t("create.industry"),
            empty: !lead.industry,
            value: (
              <InlineChoice
                label={t("create.industry")}
                hideLabel
                value={lead.industry ?? ""}
                options={[
                  { value: "", label: t("field.unset") },
                  ...industryOptions(t, locale),
                ]}
                render={(value) =>
                  value === "" ? t("field.unset") : industryWords(t, value)
                }
                canEdit={canEdit}
                readOnlyReason={reason}
                onSave={save("industry")}
              />
            ),
          },
        ]),
    {
      key: "email",
      term: t("create.email"),
      empty: isBlank(lead.email),
      value: (
        <EmailValue
          lead={lead}
          canEdit={canEdit}
          readOnlyReason={reason}
          onSave={save("email")}
        />
      ),
    },
    text("phone", "create.phone"),
    {
      key: "origin",
      term: t("lead.origin"),
      empty: !lead.origin,
      value: (
        <InlineChoice
          label={t("lead.origin")}
          hideLabel
          value={lead.origin ?? ""}
          options={originOptions}
          render={(value) =>
            value === ""
              ? t("field.unset")
              : (originLabelFor(lead, origins.data) ?? value)
          }
          canEdit={canEdit}
          readOnlyReason={reason}
          onSave={save("origin")}
        />
      ),
    },
    text("origin_detail", "lead.originDetail"),
    {
      key: "added_via",
      term: t("lead.addedVia"),
      value: addedViaFor(lead, t),
    },
    {
      key: "received",
      term: t("lead.aside.received"),
      value: formatDateAbbrev(lead.created_at, locale, viewerZone()),
    },
    // The name heads the card; the row is only here to fill a blank one.
    ...(isBlank(lead.full_name)
      ? [text("full_name", "create.fullName", { required: true })]
      : []),
    {
      key: "linkedin_url",
      term: t("create.linkedinUrl"),
      empty: isBlank(lead.linkedin_url),
      value: lead.linkedin_url ? (
        <OffsiteLink href={lead.linkedin_url}>
          {t("record.openProfile")}
        </OffsiteLink>
      ) : (
        <span title={t("record.leadProfileReadOnly")}>{t("field.unset")}</span>
      ),
    },
    text("candidate_company_key", "record.companyRoutingKey"),
    {
      key: "project_id",
      term: t("lead.project"),
      empty: !lead.project_id,
      block: !lead.project_id,
      value: lead.project_id ? (
        <EntityRef kind="project" id={lead.project_id} />
      ) : (
        <LeadProjectAttachButton
          lead={lead}
          writer={writer}
          reasonId={reasonId}
        />
      ),
    },
    {
      key: "tags",
      term: t("tags.panelTitle"),
      block: true,
      empty: (tags.data?.data ?? []).length === 0,
      value: (
        <TagsPanel
          entityType="lead"
          entityID={lead.id}
          bare
          archived={Boolean(lead.archived_at)}
          canEdit={mayTag && canEdit}
        />
      ),
    },
  ];
  return rows;
}

type DetailsProps = Readonly<{
  lead: Lead;
  writer: LeadWriter;
  terminalReasonId: string;
}>;

/**
 * The lead's details under its head: About, Ownership and Lists as flat
 * groups. Every blank folds behind one link, and a line says what a lead
 * is. The deal and project cards live on the Deals tab.
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
        found.length > 0 && (
          <ul className="lead-aside-chips">
            {found.map((list) => (
              <li key={list.id}>
                <Badge>{list.name}</Badge>
              </li>
            ))}
          </ul>
        )
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
  const about = useAboutRows(
    lead,
    writer,
    writer.readOnly ? terminalReasonId : undefined,
  );
  const setBy = statusSetBy(lead, t);
  const companyFacts = useLeadCompanyFactsSection(lead);
  const sections: DetailSection[] = [
    {
      key: "about",
      label: t("record.details.about"),
      // A lead nobody may change has no blank to fill, so it has none to show.
      rows: writer.readOnly ? about.filter((row) => !row.empty) : about,
    },
    ...(companyFacts ? [companyFacts] : []),
    {
      key: "ownership",
      label: t("record.details.ownership"),
      rows: [
        {
          key: "owner",
          term: t("lead.ownerLabel"),
          value: (
            <LeadOwnerControl
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
    // No chips, no group: the empty state is the absence of the label.
    { key: "lists", label: t("lists.record.title"), children: lists || null },
  ];
  return (
    <RecordDetails sections={sections} footnote={t("lead.aside.footnote")} />
  );
}
