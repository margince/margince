// The contact page's projects, through the one section every record uses.
//
// A contact is on a project as a STAKEHOLDER — a role on the delivery, not a
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
type ContactRole =
  components["schemas"]["SetProjectStakeholderRequest"]["role"];

type LinkedProjectRow = components["schemas"]["Company360Project"];

// What a contact can BE on a project — the delivery half of the contract's
// stakeholder vocabulary, which is what this section is for. The reader picks;
// nothing is guessed, because attaching with a guessed role OVERWRITES a role
// somebody set deliberately (the write is a PUT that re-roles an existing edge),
// and a section cannot both be one click and be safe about that.
const CONTACT_ROLES = [
  { value: "sponsor", key: "contactRole.sponsor" },
  { value: "project_lead", key: "contactRole.projectLead" },
  { value: "delivery_lead", key: "contactRole.deliveryLead" },
  { value: "subject_matter_expert", key: "contactRole.expert" },
  { value: "user", key: "contactRole.user" },
] as const;

export function ContactProjects({
  contactId,
  projects,
  readOnly,
}: Readonly<{
  contactId: string;
  projects: readonly LinkedProjectRow[] | undefined;
  readOnly?: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();

  const settled = () => {
    queryClient.invalidateQueries({ queryKey: ["contact360", contactId] });
    // The project pages this contact was attached to or detached from — keyed
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
        body: { contact_id: contactId, role: role as ContactRole },
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
        "/projects/{id}/stakeholders/{contact_id}",
        {
          params: { path: { id: projectId, contact_id: contactId } },
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
    roles: CONTACT_ROLES.map((one) => ({
      value: one.value,
      label: t(one.key),
    })),
    attach: (projectId, role) => attach.mutateAsync({ projectId, role }),
    detach: (projectId) => detach.mutateAsync(projectId),
  };

  return (
    <ProjectLinks
      adapter={adapter}
      titleKey="contactProjects.title"
      emptyBody="contactProjects.empty"
    />
  );
}
