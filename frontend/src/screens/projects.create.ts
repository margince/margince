// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { CreateField } from "../components/create";
import { useObjectCustomFields } from "../components/customfields.form";
import { useT } from "../i18n";
import { unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import {
  mapProjectCreate,
  type Project,
  type ProjectCompanyOption,
  projectFields,
} from "./projects.form";
import { useRecordOwners } from "./recordreferences";

// Which company a new project names: picked from a list on the projects
// screen, or pinned by the company page the rep is standing on.
export type ProjectCreateCompany =
  | { readonly pinned: string }
  | { readonly options: readonly ProjectCompanyOption[] };

// The one create-project form both call sites open: the core fields, the
// workspace's project custom fields, and the body the two become. A pinned
// company drops the picker and rides the body instead.
export function useProjectCreateForm(company: ProjectCreateCompany): {
  fields: CreateField[];
  create: (values: Record<string, string>) => Promise<Project>;
} {
  const t = useT();
  const cf = useObjectCustomFields("project");
  const pinned = "pinned" in company ? company.pinned : undefined;
  const me = useMe();
  const owners = useRecordOwners(null);
  const core = projectFields(t, {
    companies: "options" in company ? company.options : [],
    owners,
    me: me.data?.user.id ?? "",
    currentOwner: null,
    mode: "create",
  }).filter((field) => pinned === undefined || field.key !== "company_id");
  const create = async (values: Record<string, string>) => {
    const answers =
      pinned === undefined ? values : { ...values, company_id: pinned };
    return unwrap(
      await api.POST("/projects", {
        body: { ...mapProjectCreate(answers), ...cf.toBody(values) },
      }),
      t,
    );
  };
  return { fields: [...core, ...cf.formFields], create };
}
