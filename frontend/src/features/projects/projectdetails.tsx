// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useEmployerSummary } from "../../components/contactemployersummary";
import {
  type ObjectCustomFields,
  useCustomFieldDisplay,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { EntityRef, OwnerName } from "../../components/entityref";
import { RailField } from "../../components/railfield";
import { Badge } from "../../design-system/atoms";
import { RecordCard } from "../../design-system/recordcard";
import {
  type DetailRow,
  RecordDetails,
} from "../../design-system/recorddetails";
import { sectionState } from "../../design-system/surfacestate";
import { COMPANY_LIFECYCLE_LABELS } from "../../format/companylifecycle";
import { formatNumber } from "../../format/format";
import { formatDateAbbrev } from "../../format/format.dates";
import { industryWords } from "../../format/industry";
import { useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { useProjectEdit } from "./projectedit";
import { PHASE_LABEL, type Project } from "./projects.form";
import { timeUsed } from "./projectstanding";

// What a project is, in the details column of its page. About holds the
// account, the phase and the dates. Ownership holds the owner and who it is
// shared with. The details also carry the one rule a reader meets before
// closing it. The head's lines name the account and how far the work has run.

type Project360 = components["schemas"]["Project360"];

// Withheld and absent are different answers about a project's account: a
// project drawn with no company reads as one that has none.
function CompanyRef({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const company = view.company;
  const state = sectionState(
    view,
    "company",
    Boolean(company),
    company ? 1 : 0,
  );
  if (state === "withheld") {
    return (
      <span data-testid="project-company-withheld">{t("state.withheld")}</span>
    );
  }
  return (
    <EntityRef
      kind="company"
      id={view.project.company_id}
      name={company?.name}
    />
  );
}

/** The head's lines under the name: the account, then Day N of M. */
export function ProjectSubtitle({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const used = timeUsed(view, zone);
  return (
    <>
      <span>
        <CompanyRef view={view} />
      </span>
      {used.kind === "running" && (
        <span>
          {t("project.head.dayOf", {
            day: formatNumber(used.day, locale),
            total: formatNumber(used.total, locale),
          })}
        </span>
      )}
    </>
  );
}

/**
 * The grants on the project, read once for the head's Shared pill and the
 * Visible to row. They use the key and shape the share dialog reads, so a
 * grant made there shows here at once. Project has no visibility field of its
 * own, so who it is shared with is the answer. A refused read draws neither.
 */
function useProjectGrants(projectId: string) {
  return useQuery({
    queryKey: ["record-grants", "project", projectId],
    retry: false,
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/record-grants", {
          params: {
            query: {
              record_type: "project",
              record_id: projectId,
              limit: 100,
            },
          },
        }),
      );
      return data.data;
    },
    select: (grants) => grants.length,
  });
}

/** "Shared" in the head, only when somebody holds a grant on the project. */
export function ProjectSharedPill({
  projectId,
}: Readonly<{ projectId: string }>) {
  const t = useT();
  const grants = useProjectGrants(projectId).data;
  return grants ? <Badge>{t("project.shared")}</Badge> : null;
}

// The account as a card: its mark, name and lifecycle, and what it is.
// Withheld and absent are different answers, as in the head's line.
function CompanyCardRow({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const companyId = view.project.company_id;
  const company = view.company;
  const state = sectionState(
    view,
    "company",
    Boolean(company),
    company ? 1 : 0,
  );
  if (state === "withheld") {
    return (
      <span data-testid="project-company-withheld">{t("state.withheld")}</span>
    );
  }
  if (!companyId || !company) {
    return <span>{t("field.unset")}</span>;
  }
  return <CompanyCard companyId={companyId} name={company.name} />;
}

// A part of the line the reader may not read drops rather than reading empty.
function CompanyCard({
  companyId,
  name,
}: Readonly<{ companyId: string; name: string }>) {
  const t = useT();
  const summary = useEmployerSummary(companyId).data;
  const lifecycle =
    summary?.lifecycle && summary.lifecycle !== "unknown"
      ? t(COMPANY_LIFECYCLE_LABELS[summary.lifecycle])
      : undefined;
  const facts = [
    lifecycle,
    summary?.industry ? industryWords(t, summary.industry) : undefined,
    summary?.address?.city,
  ].filter(Boolean);
  return (
    <RecordCard
      kind="company"
      name={name}
      href={`#/companies/${companyId}`}
      identity={companyId}
      logo={summary?.logo_url}
      position={facts.length > 0 ? facts.join(" · ") : undefined}
      aside={lifecycle && <Badge wrap>{lifecycle}</Badge>}
    />
  );
}

/** The details column: About, Ownership, the empty fields and the footnote. */
export function ProjectDetails({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const cf = useObjectCustomFields("project");
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const display = useCustomFieldDisplay();
  const grants = useProjectGrants(view.project.id);
  const project = view.project;
  const unset = t("field.unset");
  const day = (date: string | null | undefined) =>
    date ? formatDateAbbrev(date, locale, zone) : unset;
  const closed = project.phase === "closed";
  const edit = useProjectEdit(project);
  const about: DetailRow[] = [
    {
      key: "company",
      term: t("project.company"),
      value: <CompanyCardRow view={view} />,
      block: true,
      empty: !project.company_id,
    },
    {
      key: "phase",
      term: t("project.phaseLabel"),
      value: t(PHASE_LABEL[project.phase]),
    },
    {
      key: "started",
      term: t("project.started"),
      value: day(project.started_at),
      empty: !project.started_at,
    },
    {
      key: "target",
      term: t("project.aside.targetEnd"),
      value: (
        <RailField
          edit={edit}
          keys={["target_end_date"]}
          label={t("project.aside.targetEnd")}
          reading={day(project.target_end_date)}
          text={project.target_end_date ? day(project.target_end_date) : ""}
        />
      ),
      empty: !project.target_end_date,
    },
    ...(closed
      ? [
          {
            key: "ended",
            term: t("project.aside.ended"),
            value: day(project.ended_at),
            empty: !project.ended_at,
          },
          {
            key: "reason",
            term: t("project.aside.closedReason"),
            value: project.closed_reason || unset,
            empty: !project.closed_reason,
          },
        ]
      : []),
    // The design's About draws no description; it stays one click away.
    {
      key: "description",
      term: t("project.description"),
      value: (
        <RailField
          edit={edit}
          keys={["description"]}
          label={t("project.description")}
          reading={project.description || unset}
          text={project.description ?? ""}
        />
      ),
      empty: true,
    },
    ...customFieldRows(cf, project, display, unset),
  ];
  const ownership: DetailRow[] = [
    {
      key: "owner",
      term: t("project.owner"),
      value: (
        <RailField
          edit={edit}
          keys={["owner_id"]}
          label={t("project.owner")}
          reading={
            <OwnerName ownerId={project.owner_id} unowned={t("list.unowned")} />
          }
        />
      ),
    },
  ];
  if (grants.data !== undefined) {
    ownership.push({
      key: "visible",
      term: t("record.details.visibleTo"),
      value:
        grants.data === 0
          ? t("record.details.notShared")
          : plural("project.visibility.sharedWith", grants.data, {
              count: formatNumber(grants.data, locale),
            }),
    });
  }
  return (
    <RecordDetails
      sections={[
        { key: "about", label: t("record.details.about"), rows: about },
        {
          key: "ownership",
          label: t("record.details.ownership"),
          rows: ownership,
        },
      ]}
      footnote={closed ? undefined : t("project.aside.closeNote")}
    />
  );
}

// A custom field with a value is an About row; an empty one folds with the
// other empty fields.
function customFieldRows(
  cf: ObjectCustomFields,
  project: Project,
  display: ReturnType<typeof useCustomFieldDisplay>,
  unset: string,
): DetailRow[] {
  const record: Record<string, unknown> = project;
  return cf.fields.map((field) => {
    const value = display(field, record[field.column_name]);
    return {
      key: field.column_name,
      term: field.label,
      value: value || unset,
      empty: !value,
    };
  });
}
