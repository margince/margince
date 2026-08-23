// The contact page's projects, through the one section every record uses.
//
// A person is on a project as a STAKEHOLDER — a role on the delivery, not a
// company working it — so the verbs here write the stakeholder edge. The
// section is the same one the company page and the project page draw, because
// "which bodies of work is this record part of" is one question however the
// record answers it.

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  ProjectLinks,
  type ProjectLinksAdapter,
} from "../design-system/projectlinks";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { searchProjects } from "./companyprojects";
import { PhaseBadge } from "./projects";
import type { ProjectPhase } from "./projects.form";

// The same row shape the company page's section takes: the two 360 reads share
// one project row, which is what lets one section serve both.
type LinkedProjectRow = components["schemas"]["Organization360Project"];

// The role a person takes when the section attaches them with no role picked.
// `user` is the weakest true claim in the contract's vocabulary: it says they
// are on the delivery without asserting they sponsor or lead it, which is what
// a one-click attach can honestly mean. A reader who needs a truer role sets it
// on the project's own stakeholder list.
const DEFAULT_STAKEHOLDER_ROLE = "user";

export function PersonProjects({
  personId,
  projects,
  readOnly,
}: Readonly<{
  personId: string;
  projects: readonly LinkedProjectRow[] | undefined;
  readOnly?: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();

  const settled = () => {
    queryClient.invalidateQueries({ queryKey: ["person360", personId] });
    queryClient.invalidateQueries({ queryKey: ["project360"] });
  };

  const attach = useMutation({
    mutationFn: async (projectId: string) => {
      const { error } = await api.PUT("/projects/{id}/stakeholders", {
        params: { path: { id: projectId } },
        body: { person_id: personId, role: DEFAULT_STAKEHOLDER_ROLE },
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
        "/projects/{id}/stakeholders/{person_id}",
        {
          params: { path: { id: projectId, person_id: personId } },
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
    allowsMany: true,
    search: searchProjects,
    attach: (projectId) => attach.mutateAsync(projectId),
    detach: (projectId) => detach.mutateAsync(projectId),
  };

  return (
    <ProjectLinks
      adapter={adapter}
      titleKey="personProjects.title"
      emptyBody="personProjects.empty"
      searchLabel={t("projectLinks.searchLabel")}
    />
  );
}
