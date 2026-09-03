// The company page's projects, through the one section every record uses.
//
// A company page had no project list at all before this: the 360 read carried
// its projects and only the meeting-brief picker looked at them. A reader on a
// company could not see which deliveries it is part of, let alone put it on
// another one.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  ProjectLinks,
  type ProjectLinksAdapter,
} from "../design-system/projectlinks";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { PhaseBadge } from "./projects";
import type { ProjectPhase } from "./projects.form";

type Organization360Project = components["schemas"]["Organization360Project"];

// What a company can BE to a project, PARTNER FIRST because the first role is
// the picker's default and a default is a claim.
//
// A project has one customer — the account the work is for — and it already has
// one by the time anybody reaches this section, since creating a project
// attaches its company as the customer. So a company joining afterwards is a
// partner or a subcontractor; defaulting it to customer would hand a project two
// customers on a reader who took the default, which is what the reports group
// by and what organization_id resolves to.
export const COMPANY_ROLES = ["partner", "subcontractor", "customer"] as const;

// The message key for one role, spelled once so the picker and any row that
// renders a role cannot disagree about the words.
export function roleKey(role: string): MessageKey {
  switch (role) {
    case "customer":
      return "projectRole.customer";
    case "subcontractor":
      return "projectRole.subcontractor";
    default:
      return "projectRole.partner";
  }
}

export function CompanyProjects({
  organizationId,
  projects,
  readOnly,
  onCreate,
  bare,
}: Readonly<{
  organizationId: string;
  projects: readonly Organization360Project[] | undefined;
  readOnly?: boolean;
  onCreate?: () => void;
  // As a group inside the pane the caller holds — see ProjectLinks.
  bare?: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();

  // Both writes invalidate the company's own 360, because that read is where
  // this section's rows come from — a section that wrote and did not refresh
  // would show the reader the state before their own change.
  const settled = () => {
    queryClient.invalidateQueries({
      queryKey: ["organization360", organizationId],
    });
    queryClient.invalidateQueries({ queryKey: ["projects"] });
    // And any project page open behind this one: the company just joined or
    // left the project it would be showing. Keyed the way project360.tsx reads
    // it — a mismatch here leaves stale rows on screen and looks like a write
    // that did not happen.
    queryClient.invalidateQueries({ queryKey: ["project"] });
  };

  const attach = useMutation({
    mutationFn: async ({
      projectId,
      role,
    }: {
      projectId: string;
      role: string;
    }) => {
      const { error } = await api.PUT("/projects/{id}/companies", {
        params: { path: { id: projectId } },
        body: { organization_id: organizationId, role },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: settled,
  });

  const detach = useMutation({
    mutationFn: async (projectId: string) => {
      const { error } = await api.DELETE(
        "/projects/{id}/companies/{organization_id}",
        {
          params: { path: { id: projectId, organization_id: organizationId } },
        },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: settled,
  });

  const adapter: ProjectLinksAdapter = {
    linked: (projects ?? []).map((project) => ({
      project_id: project.project_id,
      name: project.name,
      key: project.key,
      phase: project.phase ? (
        <PhaseBadge phase={project.phase as ProjectPhase} />
      ) : null,
    })),
    readOnly,
    // A company can be on any number of projects — that is the whole point of
    // the edge this section writes.
    allowsMany: true,
    search: searchProjects,
    roles: COMPANY_ROLES.map((value) => ({ value, label: t(roleKey(value)) })),
    attach: (projectId, role) => attach.mutateAsync({ projectId, role }),
    detach: (projectId) => detach.mutateAsync(projectId),
    onCreate,
  };

  return (
    <ProjectLinks
      adapter={adapter}
      titleKey="companyProjects.title"
      emptyBody="companyProjects.empty"
      bare={bare}
    />
  );
}

// The projects a company may be PUT on: every live one, not only this
// company's. Attaching is how a company joins a delivery it is not yet part of,
// so narrowing the search to the projects it already has would offer only the
// rows the section is already showing.
export async function searchProjects(
  query: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/projects", {
    params: { query: { q: query, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  // The key rides in the NAME because a candidate carries no second line: a
  // reader picking between two "Rollout" projects needs the handle that tells
  // them apart, and dropping it would make the list ambiguous exactly when it
  // matters.
  return (data?.data ?? []).map((project) => ({
    id: project.id,
    name: project.key ? `${project.name} · ${project.key}` : project.name,
  }));
}
