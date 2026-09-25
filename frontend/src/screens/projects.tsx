// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { Hash } from "lucide-react";
import { api } from "../api/client";
import { usePageName } from "../app/pagemeta";
import { useRecordZone } from "../app/recordzone";
import { Badge, EmptyState } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Chip } from "../design-system/readings";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { CreateAction } from "./create";
import { EntityRef } from "./entityref";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "./listquery";
import { useProjectCreateForm } from "./projects.create";
import {
  PHASE_LABEL,
  PROJECT_PHASES,
  type Project,
  type ProjectCompanyOption,
  type ProjectPhase,
} from "./projects.form";
import { lastActivityColumn, ownerColumn } from "./recordlist";
import { SaveViewAction, useSavedViews, useSavedViewTabs } from "./savedviews";
import "./projects.css";

// The projects list: every body of work the reader may see, newest activity
// first, narrowed by phase. A project starts during the deal and outlives
// close-won, which is why this list is not a column of the pipeline board —
// a project in delivery has no stage to stand in.

async function fetchProjectsPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Project>> {
  const { data, error } = await api.GET("/projects", {
    params: {
      query: {
        q: query.q || undefined,
        sort: query.sort || undefined,
        include_archived: query.includeArchived || undefined,
        cursor: cursor || undefined,
        limit: listFetchLimit(query.perPage),
        // Passed through whole rather than narrowed to a typed subset: the
        // list endpoint answers a field outside its allow-list with a 422 the
        // table renders, and a saved view the server refuses is better shown
        // refused than silently shown as an active tab over an unfiltered list.
        ...query.filters,
      },
    },
  });
  if (error) {
    throwProblem(error);
  }
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
    },
  };
}

/** The phase as a status pill: closed is the one terminal reading. */
export function PhaseBadge({ phase }: Readonly<{ phase: ProjectPhase }>) {
  const t = useT();
  return (
    <Badge tone={phase === "closed" ? undefined : "success"}>
      {t(PHASE_LABEL[phase])}
    </Badge>
  );
}

/**
 * The key as a fact chip. A key is the handle a human writes in a subject
 * line, so it draws as a fact beside the name rather than as a status.
 */
export function ProjectKeyChip({
  projectKey,
  dense,
}: Readonly<{
  projectKey: string;
  // The chip's badge geometry, for the list's name column: the key sits beside
  // the archived badge there. See `Chip`.
  dense?: boolean;
}>) {
  const t = useT();
  return (
    <Chip icon={Hash} dense={dense}>
      {/* What the key is FOR, on the chip rather than as a line of its own.
        A reader learns it once by hovering the code they are already looking
        at; a permanent sentence under the title pays every day for a lesson
        taught once, which is what it was doing. */}
      <span title={t("project.keyMinted", { key: projectKey })}>
        {projectKey}
      </span>
    </Chip>
  );
}

/**
 * The companies a project may be created on. The first page of the company
 * list, which is the same read the deal form makes for the same picker.
 */
export function useCompanyOptions(): ProjectCompanyOption[] {
  const companies = useQuery({
    queryKey: ["companies"],
    queryFn: async () => {
      const { data, error } = await api.GET("/companies", {
        params: { query: { limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  return companies.data?.data ?? [];
}

/**
 * The create affordance, shared by the list's header and its first-run plate.
 * One element rather than two renderings, so the dialog behind both is the
 * same dialog with the same fields.
 */
function NewProjectAction({
  companies,
}: Readonly<{ companies: ProjectCompanyOption[] }>) {
  const t = useT();
  const form = useProjectCreateForm({ options: companies });
  return (
    <CreateAction
      label={t("project.new")}
      invalidate="projects"
      screen="projects"
      create={form.create}
      resolveExisting={(_code, id) => ({ screen: "projects", id })}
      fields={form.fields}
    />
  );
}

const PHASE_CHIP_OPTIONS: { value: ProjectPhase; label: MessageKey }[] =
  PROJECT_PHASES.map((phase) => ({ value: phase, label: PHASE_LABEL[phase] }));

export function ProjectsScreen() {
  const t = useT();
  const pageName = usePageName("projects");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const companies = useCompanyOptions();
  const views = useSavedViews("projects");
  const savedViews = useSavedViewTabs("projects");
  const state = useListQuery<Project>({
    key: "projects",
    initialSort: "-last_activity_at",
    fetchPage: fetchProjectsPage,
  });
  // Held back until the first page has answered: the first-run plate below
  // mounts its own copy of this verb, and a button pressed in the table's
  // header a moment before the plate replaces it would open a dialog the
  // swap throws away.
  const createAction = !state.isPending && (
    <NewProjectAction companies={companies} />
  );
  // The first-run plate: nothing exists yet, and nothing is narrowing the
  // list. A filtered-empty list is the table's own case — it already knows to
  // offer the way back — and a list that is empty because a reader typed a
  // search must not be told the product has no projects. A saved view is
  // reachable only from the table's rail, so the plate waits for the views
  // read to confirm there is none; an unanswered or failed read keeps the
  // table, where SaveViewAction says what went wrong.
  const firstRun =
    !state.isPending &&
    !state.isError &&
    views.isSuccess &&
    savedViews.length === 0 &&
    state.rows.length === 0 &&
    state.query.q === "" &&
    !state.query.includeArchived &&
    Object.keys(state.query.filters).length === 0;

  if (firstRun) {
    return (
      <div className="wrap">
        {/* The page's own name, at the size and the level the populated arm
            prints it at inside the table's header. The shell prints none on
            this screen — Projects heads itself (SELF_HEADED_SCREENS), which the
            table's `title` below honours — so without this the plate REPLACED
            the page's name instead of standing under it and the route carried
            no h1 at all. A first run is exactly when a reader most needs to be
            told where they are. */}
        <Heading size="xlarge" className="projects-title t-display">
          {pageName}
        </Heading>
        <EmptyState title={t("project.emptyTitle")} action={createAction}>
          <p>{t("project.emptyBody")}</p>
          <p>{t("project.emptyKey")}</p>
        </EmptyState>
      </div>
    );
  }

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={state}
        unit="unit.projects"
        action={createAction}
        columns={[
          {
            key: "name",
            header: t("project.name"),
            cell: (project: Project) => (
              <span className="project-name-cell">
                <strong>{project.name}</strong>
                {project.key && (
                  <ProjectKeyChip projectKey={project.key} dense />
                )}
                {project.archived_at && (
                  <Badge tone="warning">{t("record.archived")}</Badge>
                )}
              </span>
            ),
            sort: "name",
            fixed: true,
          },
          {
            key: "company",
            header: t("project.company"),
            // A real link to the customer. The row opens the PROJECT, so a
            // reader who wanted the account behind it had to open the project
            // first and come back out.
            cell: (project: Project) => (
              <EntityRef kind="company" id={project.company_id} />
            ),
            // By the company's NAME. One outside this reader's scope orders
            // the page by nothing rather than by a name it withholds.
            sort: "company_id",
          },
          {
            key: "phase",
            header: t("project.phaseLabel"),
            // By how LIVE the work is — delivering, pursuing, initiative, then
            // closed — which is the arrangement the account page already uses.
            // Alphabetical would be that order shuffled.
            sort: "phase",
            cell: (project: Project) => <PhaseBadge phase={project.phase} />,
          },
          ownerColumn<Project>(t),
          lastActivityColumn<Project>(t, locale, recordZone),
        ]}
        saveView={<SaveViewAction resource="projects" query={state.query} />}
        rowKey={(project) => project.id}
        rowRoute={(project) => ({ screen: "projects", id: project.id })}
        dataViews={savedViews}
        chips={[
          {
            key: "phase",
            label: "project.phaseLabel",
            allLabel: "project.filterPhaseAll",
            options: PHASE_CHIP_OPTIONS,
          },
        ]}
        views={[
          { label: "list.viewAll", sort: "-last_activity_at" },
          {
            label: "project.viewDelivering",
            sort: "-last_activity_at",
            filters: { phase: "delivering" },
          },
        ]}
      />
    </div>
  );
}
