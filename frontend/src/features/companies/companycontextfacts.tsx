// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge, Button, Disclosure } from "../../design-system/atoms";
import { DisclosureGroup } from "../../design-system/disclosure";
import {
  EvidenceMark,
  type EvidenceMarkSource,
} from "../../design-system/evidencemark";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelRow,
} from "../../design-system/panel";
import { provenanceOf } from "../../design-system/provenanceof";
import { QueryGate, type QueryLike } from "../../design-system/querystates";
import { RowOpen } from "../../design-system/rowopen";
import { SettingRow } from "../../design-system/settingrow";
import { confidenceLevel } from "../../design-system/trust";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { coldFieldLabel } from "../../lib/coldfieldlabels";
import { SavedNotice } from "./company-context.notices";
import "./companycontextfacts.css";

type CompanyProfile = components["schemas"]["CompanyProfile"];
type CompanyInput = components["schemas"]["CompanyProfileInput"];

export const PROFILE_GROUPS = [
  {
    title: "settings.companyEssentials",
    fields: ["display_name", "offer_summary", "icp"],
  },
  {
    title: "settings.companyPositioning",
    fields: [
      "value_proposition",
      "usp",
      "customer_pains",
      "desired_outcomes",
      "buying_center",
      "buying_intents",
      "common_objections",
      "sales_motion",
    ],
  },
  {
    title: "settings.companyIdentity",
    fields: [
      "legal_name",
      "registered_address",
      "register_vat",
      "industry",
      "history",
    ],
  },
] as const satisfies readonly {
  title: MessageKey;
  fields: readonly (keyof CompanyInput)[];
}[];

// The server's required check is the first group, so those rows stand open.
const [ESSENTIALS, ...ELABORATIONS] = PROFILE_GROUPS;

// Every row opens the one editor, because one PUT writes the whole profile.
export function CompanyFactsCard({
  company,
  form,
  canEdit,
  readOnly,
  saved,
  onEdit,
}: Readonly<{
  company: QueryLike<CompanyProfile | null>;
  form: CompanyInput | null;
  canEdit: boolean;
  readOnly: boolean;
  saved: boolean;
  onEdit: (field: keyof CompanyInput) => void;
}>) {
  const t = useT();
  return (
    <Panel
      title={t("settings.companyContextTitle")}
      titleAction={
        canEdit && form ? (
          <Button
            variant="ghost"
            aria-label={t("settings.companyEditContext")}
            onClick={() => onEdit("display_name")}
          >
            {t("settings.companyEdit")}
          </Button>
        ) : null
      }
    >
      <PanelBody>
        <PanelIntro>{t("settings.companySub")}</PanelIntro>
        {/* A permission speaks once here; the rollout flag draws nothing,
            because a missing capability is not a fact about the reader. */}
        {readOnly && <p>{t("settings.companyReadOnly")}</p>}
      </PanelBody>
      <QueryGate
        query={company}
        pendingLabel={t("settings.companyContextTitle")}
      >
        {(profile) =>
          profile !== null &&
          form && (
            <>
              {ESSENTIALS.fields.map((field) => (
                <CompanyFactRow
                  key={field}
                  field={field}
                  value={String(form[field] ?? "")}
                  profile={profile}
                  canEdit={canEdit}
                  onEdit={() => onEdit(field)}
                />
              ))}
              {/* Closed, so thirteen optional facts do not bury the three the
                  save requires, and one at a time, because a reader fills them
                  in one subject at a time. */}
              <DisclosureGroup>
                {ELABORATIONS.map((group) => (
                  <Disclosure
                    key={group.title}
                    summary={
                      <GroupSummary
                        title={t(group.title)}
                        set={
                          group.fields.filter(
                            (field) => String(form[field] ?? "").trim() !== "",
                          ).length
                        }
                        total={group.fields.length}
                      />
                    }
                  >
                    {group.fields.map((field) => (
                      <CompanyFactRow
                        key={field}
                        field={field}
                        value={String(form[field] ?? "")}
                        profile={profile}
                        canEdit={canEdit}
                        onEdit={() => onEdit(field)}
                      />
                    ))}
                  </Disclosure>
                ))}
              </DisclosureGroup>
              {/* The dialog closes on save, so the confirmation lands here; a
                  refusal stays in the dialog beside the fields it refused. */}
              {saved && (
                <PanelBody>
                  <SavedNotice />
                </PanelBody>
              )}
            </>
          )
        }
      </QueryGate>
    </Panel>
  );
}

function GroupSummary({
  title,
  set,
  total,
}: Readonly<{ title: string; set: number; total: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <span className="company-fact-group">
      {title}
      <Badge>
        {plural("settings.companyGroupSet", set, {
          count: formatNumber(set, locale),
          total: formatNumber(total, locale),
        })}
      </Badge>
    </span>
  );
}

// The chevron is the keyboard path to the editor; a press elsewhere on the row
// opens it too.
function CompanyFactRow({
  field,
  value,
  profile,
  canEdit,
  onEdit,
}: Readonly<{
  field: keyof CompanyInput;
  value: string;
  profile: CompanyProfile;
  canEdit: boolean;
  onEdit: () => void;
}>) {
  const t = useT();
  const label = coldFieldLabel(field, t);
  const stored = profile.fields?.find((item) => item.field === field);
  return (
    <PanelRow
      record={canEdit}
      onOpen={canEdit ? onEdit : undefined}
      className="company-fact"
    >
      <SettingRow
        label={label}
        // A blank right column reads as a row that failed to render.
        value={
          value.trim() === "" ? (
            <span className="company-fact-unset">{t("field.unset")}</span>
          ) : (
            <EvidenceMark
              value={value}
              source={stored ? derivedSource(stored) : undefined}
            />
          )
        }
        control={null}
      />
      {canEdit && (
        <RowOpen
          // Named by the fact it changes: seventeen rows offering seventeen
          // chevrons called "Edit" make a screen reader's user count them.
          label={t("settings.companyEditField", { field: label })}
          onOpen={onEdit}
        />
      )}
    </PanelRow>
  );
}

// A typed value gets no mark, as on the record page, so the mark keeps its
// meaning.
function derivedSource(
  stored: components["schemas"]["CompanyProfileField"],
): EvidenceMarkSource | undefined {
  const provenance = provenanceOf(stored.captured_by);
  if (provenance.kind === "human") {
    return undefined;
  }
  return {
    provenance,
    confidence: confidenceLevel(stored.confidence) ?? undefined,
    snippet: stored.evidence_snippet,
    sourceUrl: stored.source_url,
  };
}
