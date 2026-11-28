// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useEmployerSummary } from "../../components/contactemployersummary";
import {
  type ObjectCustomFields,
  useCustomFieldDisplay,
  useCustomFieldLabel,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { EntityRef, OwnerName } from "../../components/entityref";
import { useSectionEdit } from "../../components/sectionedit";
import { useViewOnlyAccess } from "../../components/viewonly";
import { Badge } from "../../design-system/atoms";
import { RecordCard } from "../../design-system/recordcard";
import {
  type DetailRow,
  RecordDetails,
} from "../../design-system/recorddetails";
import { sectionState } from "../../design-system/surfacestate";
import { LIFECYCLE_LABELS } from "../../format/companylifecycle";
import { formatNumber } from "../../format/format";
import { formatDateAbbrev } from "../../format/format.dates";
import { industryWords } from "../../format/industry";
import { useLocale, usePlural, useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
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
  if (!view.project.company_id) {
    return (
      <span data-testid="project-company-none">{t("project.noCustomer")}</span>
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
  const zone = useViewerZone();
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
  if (!companyId) {
    return (
      <span data-testid="project-company-none">
        {t("project.noCustomerSet")}
      </span>
    );
  }
  if (!company) {
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
    summary?.status && summary.status !== "unknown"
      ? t(labelFor(LIFECYCLE_LABELS, summary.status))
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
      aside={
        <>
          {lifecycle && <Badge wrap>{lifecycle}</Badge>}
          {summary?.archived_at && (
            <Badge tone="warning">{t("record.archived")}</Badge>
          )}
        </>
      }
    />
  );
}

/** The details column: About, Ownership, the empty fields and the footnote. */
export function ProjectDetails({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const cf = useObjectCustomFields("project");
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const display = useCustomFieldDisplay();
  const fieldLabel = useCustomFieldLabel();
  const grants = useProjectGrants(view.project.id);
  const project = view.project;
  const unset = t("field.unset");
  const day = (date: string | null | undefined) =>
    date ? formatDateAbbrev(date, locale, zone) : unset;
  const closed = project.phase === "closed";
  const edits = useSectionEdits(project);
  const access = useViewOnlyAccess("project", project);
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
      value: t(labelFor(PHASE_LABEL, project.phase)),
    },
    {
      key: "started",
      term: t("project.started"),
      value: day(project.started_at),
      empty: !project.started_at,
    },
    {
      key: "target",
      term: t("project.targetEnd"),
      value: day(project.target_end_date),
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
    // Prose, not a line: the full width, with the writer's line breaks.
    {
      key: "description",
      term: t("project.description"),
      value: project.description || unset,
      block: Boolean(project.description),
      empty: !project.description,
    },
    ...customFieldRows(cf, project, display, unset, fieldLabel),
  ];
  const ownership: DetailRow[] = [
    {
      key: "owner",
      term: t("project.owner"),
      value: (
        <OwnerName ownerId={project.owner_id} unowned={t("list.unowned")} />
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
    <>
      <RecordDetails
        sections={[
          {
            key: "about",
            label: t("record.details.about"),
            edit: edits.about,
            rows: about,
          },
          {
            key: "ownership",
            label: t("record.details.ownership"),
            edit: edits.ownership,
            rows: ownership,
          },
        ]}
        footnote={closed ? undefined : t("project.aside.closeNote")}
        access={access}
      />
      {edits.element}
    </>
  );
}

// The other rows stay readings; Edit project holds the custom fields.
function useSectionEdits(project: Project) {
  const t = useT();
  const edit = useProjectEdit(project);
  const about = useSectionEdit(edit, {
    label: t("record.details.about"),
    // A project gets its end day once it is closed, as its row does.
    keys: [
      "started_at",
      "target_end_date",
      ...(project.phase === "closed" ? ["ended_at"] : []),
      "description",
    ],
  });
  const ownership = useSectionEdit(edit, {
    label: t("record.details.ownership"),
    keys: ["owner_id"],
  });
  return {
    about: about.edit,
    ownership: ownership.edit,
    element: (
      <>
        {about.element}
        {ownership.element}
      </>
    ),
  };
}

// A custom field with a value is an About row; an empty one folds with the
// other empty fields.
function customFieldRows(
  cf: ObjectCustomFields,
  project: Project,
  display: ReturnType<typeof useCustomFieldDisplay>,
  unset: string,
  fieldLabel: ReturnType<typeof useCustomFieldLabel>,
): DetailRow[] {
  const record: Record<string, unknown> = project;
  return cf.fields.map((field) => {
    const value = display(field, record[field.column_name]);
    return {
      key: field.column_name,
      term: fieldLabel(field),
      value: value || unset,
      empty: !value,
    };
  });
}
