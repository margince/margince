// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Eyebrow } from "../../design-system/eyebrow";
import { useT } from "../../i18n";
import { factCategoryLabelKey, factFieldLabelKey } from "../factview";
import type { CompanyFieldName } from "../onboarding";
import type { ReviewRow } from "./company-review-state";
import {
  type Citation,
  citationOf,
  type Fact,
  type LegalEntity,
  type Page,
  type Person,
  pageKindLabelKey,
  pageOf,
  referenceAddressOf,
} from "./profile-digest-data";
import {
  DigestLine,
  FactLine,
  LegalEntityLine,
  PersonLine,
} from "./profile-digest-lines";
import { articleSections } from "./profile-digest-sections";

// The whole-record document's left column: the record grouped into the
// sections a reader recognises from reading it rather than from filling it in
// — see profile-digest-sections.ts for why the grouping is the form's own
// four clusters under different words — followed by what the crawl found
// beyond those fields, and the numbered pages every line above cited.

export function ProfileArticle({
  rows,
  number,
  pages,
  legalEntities,
  factGroups,
  people,
  cites,
  onSettle,
}: Readonly<{
  rows: readonly ReviewRow[];
  number: ReadonlyMap<string, number>;
  pages: readonly Page[];
  legalEntities: readonly LegalEntity[];
  factGroups: ReadonlyArray<{
    category: Fact["category"];
    facts: readonly Fact[];
  }>;
  people: readonly Person[];
  cites: readonly Citation[];
  /** Undefined draws every unanswered row as a plain blank rather than an
   * action — the digest's own optional contract, kept the same here. */
  onSettle?: (field: CompanyFieldName) => void;
}>) {
  const t = useT();
  const byField = new Map(rows.map((row) => [row.field, row]));

  return (
    <div className="pdigest-article">
      {articleSections().map((section) => {
        const sectionRows = section.fields
          .map((field) => byField.get(field))
          .filter((row): row is ReviewRow => row !== undefined);
        const entities = section.key === "identity" ? legalEntities : [];
        if (sectionRows.length === 0 && entities.length === 0) {
          return null;
        }
        return (
          <section key={section.key} className="pdigest-section">
            <Eyebrow as="h3" className="pdigest-heading">
              {t(section.labelKey)}
            </Eyebrow>
            {sectionRows.map((row) => (
              <DigestLine
                key={row.field}
                row={row}
                n={citationOf(row, number)}
                onSettle={onSettle}
              />
            ))}
            {entities.map((entity) => (
              <LegalEntityLine
                // The name alone collides when two pages print the same
                // entity; the page it came from is what tells them apart.
                key={`${entity.name}@${entity.source_url}`}
                entity={entity}
                n={number.get(entity.source_url)}
              />
            ))}
          </section>
        );
      })}
      {factGroups.length === 0 ? null : (
        <section className="pdigest-section">
          <Eyebrow as="h3" className="pdigest-heading">
            {t("ob.digest.facts")}
          </Eyebrow>
          {factGroups.map((group) => (
            <div key={group.category} className="pdigest-factgroup">
              <p className="pdigest-subhead">
                {t(factCategoryLabelKey(group.category))}
              </p>
              {group.facts.map((fact) => (
                <FactLine
                  key={`${fact.field}:${fact.value_key}`}
                  fact={fact}
                  label={t(factFieldLabelKey(fact.field))}
                  n={number.get(fact.evidence_url)}
                />
              ))}
            </div>
          ))}
        </section>
      )}
      {people.length === 0 ? null : (
        <section className="pdigest-section">
          <Eyebrow as="h3" className="pdigest-heading">
            {t("ob.digest.people")}
          </Eyebrow>
          {people.map((person) => (
            <PersonLine
              key={`${person.name}:${person.role}@${person.evidence_url}`}
              person={person}
              n={number.get(person.evidence_url)}
            />
          ))}
        </section>
      )}
      {cites.length === 0 ? null : (
        <section className="pdigest-sources">
          <Eyebrow as="h3" className="pdigest-heading">
            {t("ob.digest.sources")}
          </Eyebrow>
          <ol>
            {cites.map((cite) => (
              <li key={cite.url}>
                <span className="pdigest-source-n">{cite.n}</span>
                <span className="pdigest-source-path">
                  {referenceAddressOf(cite.url)}
                </span>
                <span className="pdigest-source-label">
                  {t(pageKindLabelKey(pageOf(pages, cite.url)?.kind))}
                </span>
              </li>
            ))}
          </ol>
          <p className="pdigest-source-note">{t("ob.digest.referenceNote")}</p>
        </section>
      )}
    </div>
  );
}
