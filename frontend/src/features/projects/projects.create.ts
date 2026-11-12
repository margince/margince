// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { CreateField } from "../../components/create";
import { useObjectCustomFields } from "../../components/customfields.form";
import { useOwnerPicker } from "../../components/recordowners";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import {
  mapProjectCreate,
  type Project,
  projectCreateFields,
  projectFields,
} from "./projects.form";

// The one create-project form both call sites open. It holds the core fields,
// the suggested key, the starting phase, the deal and the custom fields.
// A company pinned by the company page drops the picker and rides the body.
export function useProjectCreateForm({
  pinned,
}: Readonly<{ pinned?: string }> = {}): {
  fields: CreateField[];
  create: (values: Record<string, string>) => Promise<Project>;
  onValuesChange: (values: Record<string, string>) => void;
} {
  const t = useT();
  const cf = useObjectCustomFields("project", "create");
  const owner = useOwnerPicker(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const suggestedKey = useKeyPreview(values.name ?? "");
  const company = pinned ?? values.company_id ?? "";
  const deals = useDealsOfCompany(company);
  const core = projectFields(t, {
    owner,
    mode: "create",
  }).filter((field) => pinned === undefined || field.key !== "company_id");
  const extras = projectCreateFields(t, { suggestedKey, deals });
  const create = async (submitted: Record<string, string>) => {
    const answers =
      pinned === undefined ? submitted : { ...submitted, company_id: pinned };
    return unwrap(
      await api.POST("/projects", {
        body: { ...mapProjectCreate(answers), ...cf.toBody(submitted) },
      }),
      t,
    );
  };
  return {
    fields: [...core, ...extras, ...cf.formFields],
    create,
    onValuesChange: setValues,
  };
}

// The key a create of this name would mint, asked of the server once the name
// stops changing. Undefined while there is no name or no answer yet.
function useKeyPreview(name: string): string | undefined {
  const settled = useSettledValue(name.trim());
  const preview = useQuery({
    queryKey: ["projects", "key-preview", settled],
    queryFn: async () =>
      unwrap(
        await api.GET("/projects/key-preview", {
          params: { query: { name: settled } },
        }),
      ).key,
    enabled: settled !== "",
    staleTime: 10_000,
  });
  return settled === "" ? undefined : preview.data;
}

// The open deals of the company the form names, as picker options. The server
// refuses a deal on another company, so the picker offers none until a
// company is chosen.
function useDealsOfCompany(
  companyId: string,
): readonly { value: string; label: string }[] {
  const deals = useQuery({
    queryKey: ["deals", "of-company", companyId],
    queryFn: async () =>
      unwrap(
        await api.GET("/deals", {
          params: { query: { company_id: companyId, limit: 100 } },
        }),
      ).data,
    enabled: companyId !== "",
    staleTime: 60_000,
  });
  return (deals.data ?? [])
    .filter((deal) => !deal.project_id)
    .map((deal) => ({ value: deal.id, label: deal.name }));
}
