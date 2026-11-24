// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { omitted } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { NewDealAction } from "../companies/companyactions";
import type { Project360 } from "./projectsections";

// New deal on a project binds the deal to the project's customer company. With
// no customer, the reader is told why instead of shown a button that fails.
export function ProjectNewDeal({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const canCreateDeal = useCanWrite("deal", "create");
  const project = view.project;
  if (!canCreateDeal) return null;
  if (project.company_id) {
    return (
      <NewDealAction
        companyId={project.company_id}
        companyName={view.company?.name ?? project.name}
        projectId={project.id}
        link
      />
    );
  }
  if (omitted(view, "company")) return null;
  return (
    <span data-testid="project-new-deal-needs-company">
      {t("project.newDealNeedsCustomer")}
    </span>
  );
}
