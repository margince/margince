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
type PersonRole = components["schemas"]["SetProjectStakeholderRequest"]["role"];

type LinkedProjectRow = components["schemas"]["Organization360Project"];

// What a person can BE on a project — the delivery half of the contract's
// stakeholder vocabulary, which is what this section is for. The reader picks;
// nothing is guessed, because attaching with a guessed role OVERWRITES a role
// somebody set deliberately (the write is a PUT that re-roles an existing edge),
// and a section cannot both be one click and be safe about that.
const PERSON_ROLES = [
  { value: "sponsor", key: "personRole.sponsor" },
  { value: "project_lead", key: "personRole.projectLead" },
  { value: "delivery_lead", key: "personRole.deliveryLead" },
  { value: "subject_matter_expert", key: "personRole.expert" },
  { value: "user", key: "personRole.user" },
] as const;

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
    // The project pages this person was attached to or detached from — keyed
    // the way project360.tsx reads them.
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
      const { error } = await api.PUT("/projects/{id}/stakeholders", {
        params: { path: { id: projectId } },
        body: { person_id: personId, role: role as PersonRole },
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
    roles: PERSON_ROLES.map((one) => ({ value: one.value, label: t(one.key) })),
    attach: (projectId, role) => attach.mutateAsync({ projectId, role }),
    detach: (projectId) => detach.mutateAsync(projectId),
  };

  return (
    <ProjectLinks
      adapter={adapter}
      titleKey="personProjects.title"
      emptyBody="personProjects.empty"
    />
  );
}
