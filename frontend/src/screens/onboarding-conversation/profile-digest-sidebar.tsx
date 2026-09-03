// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Eyebrow } from "../../design-system/eyebrow";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ReviewRow } from "./company-review-state";
import {
  bestFact,
  citationOf,
  type Fact,
  hostOf,
  type LegalEntity,
} from "./profile-digest-data";

// The whole-record document's right column: a fixed set of facts a reader
// scans without opening the article, each sourced from data the record or the
// read already carries. A pair whose underlying datum does not exist at all —
// no fact of that kind ever came back — is left out rather than shown empty;
// a pair backed by a record field always shows, "not written down" and all,
// because that field's slot exists on every record whether or not this one
// filled it.

/** One sidebar row, before it is known whether it has anything to show. */
type Candidate = Readonly<{
  labelKey: MessageKey;
  value: string | undefined;
  n?: number;
  /** False for a row backed by a record field, which always has a slot to
   * report on even when it is empty; true for one backed only by a fact,
   * which has nothing to say at all when no such fact came back. */
  omitWhenAbsent: boolean;
}>;

function joinValues(facts: readonly Fact[], field: Fact["field"]): string {
  return facts
    .filter((fact) => fact.field === field)
    .map((fact) => fact.value)
    .join(", ");
}

export function ProfileSidebar({
  rows,
  number,
  facts,
  legalEntities,
  rootUrl,
}: Readonly<{
  rows: readonly ReviewRow[];
  number: ReadonlyMap<string, number>;
  facts: readonly Fact[];
  legalEntities: readonly LegalEntity[];
  rootUrl: string;
}>) {
  const t = useT();
  const byField = new Map(rows.map((row) => [row.field, row]));
  // Only a notice that names ONE entity can stand in for a blank record line.
  // A group's notice names several, and the read does not decide which one
  // this installation is (LegalEntityLine says the same), so the sidebar
  // must not either: printing the first would state another company's name
  // as a fact about this record.
  const soleEntity = legalEntities.length === 1 ? legalEntities[0] : undefined;
  const entityValue = (value: string | undefined): EntityValue | undefined =>
    soleEntity === undefined || value === undefined
      ? undefined
      : { value, n: number.get(soleEntity.source_url) };

  const legalName = rowOrEntity(
    byField.get("legal_name"),
    entityValue(soleEntity?.name),
    number,
    "ob.digest.sidebar.legalName",
  );
  const headquarters = rowOrEntity(
    byField.get("registered_address"),
    entityValue(soleEntity?.registered_address),
    number,
    "ob.digest.sidebar.headquarters",
  );
  const industryRow = byField.get("industry");

  const founded = bestFact(facts, "founded_year");
  const employees = bestFact(facts, "employee_range");
  const offices = joinValues(facts, "location");
  const officesSource = bestFact(facts, "location");
  const certifications = joinValues(facts, "certification");
  const certificationsSource = bestFact(facts, "certification");

  const candidates: readonly Candidate[] = [
    legalName,
    {
      labelKey: "ob.digest.sidebar.founded",
      value: founded?.value,
      n: founded && number.get(founded.evidence_url),
      omitWhenAbsent: true,
    },
    headquarters,
    {
      labelKey: "ob.digest.sidebar.offices",
      value: offices === "" ? undefined : offices,
      n: officesSource && number.get(officesSource.evidence_url),
      omitWhenAbsent: true,
    },
    {
      labelKey: "ob.field.industry",
      value: industryRow?.value,
      n: industryRow && citationOf(industryRow, number),
      omitWhenAbsent: false,
    },
    {
      labelKey: "ob.digest.sidebar.employees",
      value: employees?.value,
      n: employees && number.get(employees.evidence_url),
      omitWhenAbsent: true,
    },
    {
      // The site under discussion, not a claim it cites — no superscript.
      labelKey: "org.website",
      value: hostOf(rootUrl),
      omitWhenAbsent: false,
    },
    {
      labelKey: "ob.digest.sidebar.certifications",
      value: certifications === "" ? undefined : certifications,
      n: certificationsSource && number.get(certificationsSource.evidence_url),
      omitWhenAbsent: true,
    },
  ];

  const visible = candidates.filter(
    (candidate) => candidate.value !== undefined || !candidate.omitWhenAbsent,
  );

  return (
    <aside className="pdigest-sidebar" aria-label={t("ob.digest.sidebarLabel")}>
      <dl className="pdigest-side-list">
        {visible.map((candidate) => (
          <SidebarRow key={candidate.labelKey} {...candidate} />
        ))}
      </dl>
    </aside>
  );
}

/** What the legal notice says, and the number of the page that says it —
 * cited on the same rule as every other line here. */
type EntityValue = Readonly<{ value: string; n: number | undefined }>;

/** The record's own line first; the notice's entity only where the record is
 * still blank. Either way the slot exists, so the row always shows. */
function rowOrEntity(
  row: ReviewRow | undefined,
  entity: EntityValue | undefined,
  number: ReadonlyMap<string, number>,
  labelKey: MessageKey,
): Candidate {
  const rowHasValue = row !== undefined && row.value.trim() !== "";
  if (rowHasValue) {
    return {
      labelKey,
      value: row.value,
      n: citationOf(row, number),
      omitWhenAbsent: false,
    };
  }
  return {
    labelKey,
    value: entity?.value,
    n: entity?.n,
    omitWhenAbsent: false,
  };
}

function SidebarRow({
  labelKey,
  value,
  n,
}: Readonly<{ labelKey: MessageKey; value: string | undefined; n?: number }>) {
  const t = useT();
  return (
    <div className="pdigest-side-row">
      <Eyebrow as="dt" className="pdigest-side-label">
        {t(labelKey)}
      </Eyebrow>
      <dd className="pdigest-side-value">
        {value === undefined || value === "" ? (
          <span className="pdigest-blank">{t("ob.digest.notWritten")}</span>
        ) : (
          <>
            {value}
            {n === undefined ? null : <sup className="pdigest-ref">{n}</sup>}
          </>
        )}
      </dd>
    </div>
  );
}
