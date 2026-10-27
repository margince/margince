// The project page's companies, through the same section every record uses for
// its project links — pointed the other way.
//
// A project is work several companies do together, so this is where a reader
// sees who is on it and puts another company on. It is the mirror of the
// company page's projects list, and deliberately the same control: two lists
// answering "who is working this together" would drift the first time one of
// them grew a verb.

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  CompanyMark,
  employerFacts,
  hasEmployerFacts,
  useEmployerSummary,
} from "../../components/contactemployersummary";
import { Badge, Button, OverflowMenu } from "../../design-system/atoms";
import { CompanyCard } from "../../design-system/companycard";
import {
  type LinkedProject,
  ProjectLinks,
  type ProjectLinksAdapter,
} from "../../design-system/projectlinks";
import type { RecordPickerCandidate } from "../../design-system/recordpicker";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import {
  COMPANY_ROLE_LABEL,
  COMPANY_ROLES,
  companyRoleLabel,
} from "../companies/companyprojects";
import { project360Key } from "./project360.key";

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
    queryClient.invalidateQueries({ queryKey: project360Key(projectId) });
    queryClient.invalidateQueries({ queryKey: ["company360"] });
  };

  const attach = useMutation({
    mutationFn: async ({
      companyId,
      role,
    }: {
      companyId: string;
      role: string;
    }) => {
      unwrap(
        await api.PUT("/projects/{id}/companies", {
          params: { path: { id: projectId } },
          body: { company_id: companyId, role },
        }),
      );
    },
    onSuccess: settled,
  });

  const detach = useMutation({
    mutationFn: async (companyId: string) => {
      unwrap(
        await api.DELETE("/projects/{id}/companies/{company_id}", {
          params: { path: { id: projectId, company_id: companyId } },
        }),
      );
    },
    onSuccess: settled,
  });

  // A company's row on a project is shaped like a project's row on a company:
  // the same three slots, with the ROLE standing where a phase would. That is
  // what makes one component serve both — each row says what this record is to
  // the other one. The card draws it the way a contact's employers are drawn.
  const adapter: ProjectLinksAdapter = {
    linked: (companies ?? []).map((company) => {
      const roleLabel = companyRoleLabel(company.role);
      return {
        project_id: company.company_id,
        name: company.display_name,
        phase: <Badge>{roleLabel ? t(roleLabel) : company.role}</Badge>,
        href: `#/companies/${company.company_id}`,
      };
    }),
    readOnly,
    card: (company, onDetach) => (
      <ProjectCompanyCard company={company} onDetach={onDetach} />
    ),
    allowsMany: true,
    search: searchCompanies,
    // The same three roles the company page offers, from the other side: one
    // vocabulary, so a company attached here and one attached there mean the
    // same thing.
    roles: COMPANY_ROLES.map((value) => ({
      value,
      label: t(COMPANY_ROLE_LABEL[value]),
    })),
    attach: (companyId, role) => attach.mutateAsync({ companyId, role }),
    detach: (companyId) => detach.mutateAsync(companyId),
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
        detachConfirm: t("projectCompanies.detachConfirm"),
        search: t("projectCompanies.searchLabel"),
      }}
    />
  );
}

function ProjectCompanyCard({
  company,
  onDetach,
}: Readonly<{ company: LinkedProject; onDetach: (() => void) | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const summary = useEmployerSummary(company.project_id).data;
  return (
    <CompanyCard
      logo={
        <CompanyMark
          companyId={company.project_id}
          name={company.name}
          logoUrl={summary?.logo_url}
        />
      }
      title={<a href={company.href}>{company.name}</a>}
      badges={company.phase}
      facts={
        summary && hasEmployerFacts(summary)
          ? employerFacts(summary, t, locale)
          : null
      }
      actions={
        onDetach && (
          <OverflowMenu label={t("record.moreActions")}>
            <Button
              variant="danger"
              onClick={onDetach}
              // Five identical "Detach" items cannot say which company each
              // one takes off.
              aria-label={t("projectLinks.detachNamed", { name: company.name })}
            >
              {t("projectLinks.detach")}
            </Button>
          </OverflowMenu>
        )
      }
    />
  );
}

async function searchCompanies(
  query: string,
): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/companies", {
      params: { query: { q: query, limit: 10 } },
    }),
  );
  return (data?.data ?? []).map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}
