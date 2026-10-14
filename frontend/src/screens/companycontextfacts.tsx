// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, Button, Disclosure } from "../design-system/atoms";
import { DisclosureGroup } from "../design-system/disclosure";
import {
  EvidenceMark,
  type EvidenceMarkSource,
} from "../design-system/evidencemark";
import { Panel, PanelBody, PanelIntro, PanelRow } from "../design-system/panel";
import { provenanceOf } from "../design-system/provenanceof";
import { QueryGate, type QueryLike } from "../design-system/querystates";
import { RowOpen } from "../design-system/rowopen";
import { SettingRow } from "../design-system/settingrow";
import { confidenceLevel } from "../design-system/trust";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { coldFieldLabel } from "./coldfieldlabels";
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

// The three the save demands, and the two groups that say more. The server's
// required check is this first group, so those rows stand open. All three are
// still one form in the dialog, because one PUT writes the whole profile.
const [ESSENTIALS, ...ELABORATIONS] = PROFILE_GROUPS;

/**
 * What we hold, as one row per fact. Every row opens the one editor, because
 * one PUT writes all of them. A value the site read carries a provenance mark.
 */
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
                  <div className="settings-panel-commit">
                    <SavedNotice />
                  </div>
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

/**
 * What we hold for one field, as a row: the fact named and what it says. The
 * row's chevron is the keyboard path to the editor; a press anywhere else on
 * the row opens it too.
 */
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
        // An empty field says so, in the words every other surface uses for it.
        // A blank right column is indistinguishable from a row that failed to
        // render its answer.
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

/**
 * Where a value came from, when a contact did not type it. A typed value gets
 * no mark, as on the record page, so the mark keeps its meaning. It carries no
 * date: this page has no record zone to read a timestamp against.
 */
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
