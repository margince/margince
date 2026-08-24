// The project page's companies, through the same section every record uses for
// its project links — pointed the other way.
//
// A project is work several companies do together, so this is where a reader
// sees who is on it and puts another company on. It is the mirror of the
// company page's projects list, and deliberately the same control: two lists
// answering "who is working this together" would drift the first time one of
// them grew a verb.

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import {
  ProjectLinks,
  type ProjectLinksAdapter,
} from "../design-system/projectlinks";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { COMPANY_ROLES, roleKey } from "./companyprojects";

type ProjectCompany = components["schemas"]["ProjectCompany"];

export function ProjectCompanies({
  projectId,
  companies,
  readOnly,
}: Readonly<{
  projectId: string;
  companies: readonly ProjectCompany[] | undefined;
  readOnly?: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();

  const settled = () => {
    // The key the project page actually reads under (project360.tsx), not a
    // guess: a mismatch leaves the section showing the state before the write.
    queryClient.invalidateQueries({ queryKey: ["project", projectId, "360"] });
    queryClient.invalidateQueries({ queryKey: ["organization360"] });
  };

  const attach = useMutation({
    mutationFn: async ({
      organizationId,
      role,
    }: {
      organizationId: string;
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
    mutationFn: async (organizationId: string) => {
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

  // A company's row on a project is shaped like a project's row on a company:
  // the same three slots, with the ROLE standing where a phase would. That is
  // what makes one component serve both — each row says what this record is to
  // the other one.
  const adapter: ProjectLinksAdapter = {
    linked: (companies ?? []).map((company) => ({
      project_id: company.organization_id,
      name: company.display_name,
      phase: <Badge>{company.role}</Badge>,
      href: `#/companies/${company.organization_id}`,
    })),
    readOnly,
    allowsMany: true,
    search: searchCompanies,
    // The same three roles the company page offers, from the other side: one
    // vocabulary, so a company attached here and one attached there mean the
    // same thing.
    roles: COMPANY_ROLES.map((value) => ({ value, label: t(roleKey(value)) })),
    attach: (organizationId, role) =>
      attach.mutateAsync({ organizationId, role }),
    detach: (organizationId) => detach.mutateAsync(organizationId),
  };

  return (
    <ProjectLinks
      adapter={adapter}
      titleKey="projectCompanies.title"
      emptyBody="projectCompanies.empty"
      // The mirror links COMPANIES, so every word it shows says so — the verb
      // on screen and the dialog's accessible name alike.
      words={{
        attach: t("projectCompanies.attach"),
        move: t("projectCompanies.attach"),
        detachTitle: t("projectCompanies.detachTitle"),
        search: t("projectCompanies.searchLabel"),
      }}
    />
  );
}

async function searchCompanies(
  query: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/organizations", {
    params: { query: { q: query, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return (data?.data ?? []).map((org) => ({
    id: org.id,
    name: org.display_name,
  }));
}
