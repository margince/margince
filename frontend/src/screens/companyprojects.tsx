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
import { throwProblem } from "./common";
import { PhaseBadge } from "./projects";
import type { ProjectPhase } from "./projects.form";

type Organization360Project = components["schemas"]["Organization360Project"];

export function CompanyProjects({
  organizationId,
  projects,
  readOnly,
  onCreate,
}: Readonly<{
  organizationId: string;
  projects: readonly Organization360Project[] | undefined;
  readOnly?: boolean;
  onCreate?: () => void;
}>) {
  const queryClient = useQueryClient();

  // Both writes invalidate the company's own 360, because that read is where
  // this section's rows come from — a section that wrote and did not refresh
  // would show the reader the state before their own change.
  const settled = () => {
    queryClient.invalidateQueries({
      queryKey: ["organization360", organizationId],
    });
    queryClient.invalidateQueries({ queryKey: ["projects"] });
  };

  const attach = useMutation({
    mutationFn: async (projectId: string) => {
      const { error } = await api.PUT("/projects/{id}/companies", {
        params: { path: { id: projectId } },
        body: { organization_id: organizationId, role: "partner" },
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
    attach: (projectId) => attach.mutateAsync(projectId),
    detach: (projectId) => detach.mutateAsync(projectId),
    onCreate,
  };

  return (
    <ProjectLinks
      adapter={adapter}
      titleKey="companyProjects.title"
      emptyBody="companyProjects.empty"
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
