// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import type { components } from "../../api/schema";
import {
  addressFrom,
  companyEditFields,
  mapCompanyUpdate,
  searchCompanyTargets,
} from "../../components/companyform";
import {
  type CreateField,
  joinMultiselectValue,
} from "../../components/create";
import { prefillRowsFromRecord } from "../../components/edit.prefill";
import { useEntityName } from "../../components/entityref";
import type { RailEdit } from "../../components/railfield";
import { saveRecordEdit } from "../../components/recordedit";
import { type FieldRecord, rawRecord } from "../../components/recordfieldtypes";
import { useLocale, useT } from "../../i18n";
import { useIndustryOptions } from "../../lib/industryoptions";
import { invalidateCompany } from "../../lib/recordwritekeys";

type Company = components["schemas"]["Company"];
type UpdateCompanyRequest = components["schemas"]["UpdateCompanyRequest"];

/**
 * One write to the company's own row, through the conditional edit every
 * field on the page uses, then the reads that show it.
 */
export function useCompanyFieldSave(company: Company) {
  const queryClient = useQueryClient();
  return useCallback(
    async (patch: UpdateCompanyRequest) => {
      await saveRecordEdit("company", company, patch);
      await invalidateCompany(queryClient, company);
    },
    [company, queryClient],
  );
}

/** The account's own fields. The owner is not one: it changes through `useCompanyOwnerFlow`. */
export function useCompanyEdit(company: Company, canEdit: boolean): RailEdit {
  const t = useT();
  const { locale } = useLocale();
  const industries = useIndustryOptions(company.industry);
  const parent = useEntityName("company", company.parent_company_id);
  const fields = companyEditFields(
    [],
    false,
    t,
    locale,
    company.address?.country,
  ).flatMap((field): CreateField[] => {
    if (field.key === "owner_id") return [];
    if (field.key === "industry") return [{ ...field, options: industries }];
    // The wire may omit the stage, and a required blank would hold every
    // other About field hostage to it.
    if (field.key === "status")
      return [{ ...field, required: Boolean(company.status) }];
    return [field];
  });
  return {
    kind: "company",
    canEdit,
    record: fieldRecord(company),
    fields: [...fields, parentField(company, parent.name ?? undefined)],
    save: (values, rows, opened) =>
      saveRecordEdit(
        "company",
        rawRecord(opened),
        mapCompanyUpdate(values, rows, openedDomains(opened)),
      ),
  };
}

function parentField(company: Company, parentName: string | undefined) {
  return {
    key: "parent_company_id",
    label: "history.field.parent_company_id",
    searchTargets: async (q: string) =>
      (await searchCompanyTargets(q)).filter(
        (candidate) => candidate.id !== company.id,
      ),
    options: company.parent_company_id
      ? [
          {
            value: company.parent_company_id,
            label: parentName ?? company.parent_company_id,
          },
        ]
      : [],
  } satisfies CreateField;
}

function fieldRecord(company: Company): FieldRecord {
  return {
    ...company,
    original: company,
    parent_company_id: company.parent_company_id ?? "",
    relationship_types: joinMultiselectValue(company.relationship_types ?? []),
    ...addressFrom(company.address),
    domains: (company.domains ?? []).map((domain) => ({
      domain: domain.domain,
      is_primary: String(domain.is_primary),
    })),
  };
}

// The domains a save is a replace-set against: the ones the form opened on.
function openedDomains(opened: FieldRecord) {
  const rows = prefillRowsFromRecord(
    [{ key: "domains", type: "repeatable" }],
    opened,
  ).domains;
  return (rows ?? []).map((domain) => ({
    domain: domain.domain ?? "",
    is_primary: domain.is_primary === "true",
  }));
}
