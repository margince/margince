import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { sectionState } from "../design-system/surfacestate";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import { EntityRef, OwnerName } from "./entityref";

// The project's identity line. Extracted from project360.tsx so that file
// stays under the length cap; the page imports it and renders it where it
// always sat, under the record name.

type Project360 = components["schemas"]["Project360"];

/** The company and the owner, under the name. */
export function ProjectSubtitle({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const project = view.project;
  const company = view.company;
  const companyState = sectionState(
    view,
    "company",
    Boolean(company),
    company ? 1 : 0,
  );
  return (
    <span className="project-subtitle">
      {companyState === "withheld" ? (
        // The grant refused the company: say so rather than leave the name
        // out, which would read as a project with no company.
        <span data-testid="project-company-withheld">
          {t("state.withheld")}
        </span>
      ) : (
        <EntityRef
          kind="company"
          id={project.company_id}
          name={company?.name}
        />
      )}
      <span aria-hidden="true">·</span>
      <OwnerName ownerId={project.owner_id} unowned={t("list.unowned")} />
      {project.target_end_date && (
        <>
          <span aria-hidden="true">·</span>
          <span>
            {t("project.targetEndShort", {
              date: formatDate(project.target_end_date, locale, recordZone),
            })}
          </span>
        </>
      )}
    </span>
  );
}
