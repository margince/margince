// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { CreateField } from "../../components/create";
import { useObjectCustomFields } from "../../components/customfields.form";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useOwnerPicker } from "../records/recordreferences";
import { mapProjectCreate, type Project, projectFields } from "./projects.form";

// The one create-project form both call sites open: the core fields, the
// workspace's project custom fields, and the body the two become. The company
// is searched on the projects screen. A company pinned by the company page
// drops the picker and rides the body instead.
export function useProjectCreateForm({
  pinned,
}: Readonly<{ pinned?: string }> = {}): {
  fields: CreateField[];
  create: (values: Record<string, string>) => Promise<Project>;
} {
  const t = useT();
  const cf = useObjectCustomFields("project", "create");
  const owner = useOwnerPicker(null);
  const core = projectFields(t, {
    owner,
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
