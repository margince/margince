// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useCallback, useState } from "react";
import type { components } from "../../api/schema";
import {
  ADDRESS_FIELDS,
  addressFrom,
  companyEditFields,
  mapCompanyUpdate,
  searchCompanyTargets,
} from "../../components/companyform";
import type { CreateField, FormRows } from "../../components/create";
import { joinMultiselectValue } from "../../components/create";
import { EditRecordModal } from "../../components/edit";
import { prefillRowsFromRecord } from "../../components/edit.prefill";
import { useEntityName } from "../../components/entityref";
import { saveRecordEdit } from "../../components/recordedit";
import { Button } from "../../design-system/atoms";
import { useLocale, useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { problemMessageOf } from "../../lib/problem";
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

// The fields a details row edits in a dialog rather than in place. Each is
// several inputs (an address, a set of domains) or a search.
export type CompoundField =
  | "address"
  | "domains"
  | "parent_company_id"
  | "relationship_types";

function compoundFields(
  which: CompoundField,
  company: Company,
  parentName: string | undefined,
  t: ReturnType<typeof useT>,
  locale: Locale,
): CreateField[] {
  if (which === "address") {
    return ADDRESS_FIELDS;
  }
  if (which === "parent_company_id") {
    return [
      {
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
      },
    ];
  }
  return companyEditFields([], false, t, locale).filter(
    (field) => field.key === which,
  );
}

function fieldRecord(company: Company) {
  return {
    original: company,
    id: company.id,
    version: company.version,
    parent_company_id: company.parent_company_id ?? "",
    relationship_types: joinMultiselectValue(company.relationship_types ?? []),
    ...addressFrom(company.address),
    domains: (company.domains ?? []).map((domain) => ({
      domain: domain.domain,
      is_primary: String(domain.is_primary),
    })),
  };
}

/**
 * A details value that opens a small form: the reading as the trigger, the
 * company's own edit dialog for just that field behind it.
 */
export function CompoundFieldValue({
  company,
  which,
  label,
  reading,
  canEdit,
  readOnlyReason,
}: Readonly<{
  company: Company;
  which: CompoundField;
  label: string;
  // What the row shows at rest; absent reads as "Not set".
  reading?: ReactNode;
  canEdit: boolean;
  readOnlyReason?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const save = useCompanyFieldSave(company);
  const parent = useEntityName("company", company.parent_company_id);
  const mutation = useMutation({
    mutationFn: async ({
      values,
      rows,
      opened,
    }: {
      values: Record<string, string>;
      rows: FormRows;
      opened: Record<string, unknown>;
    }) => {
      const current = prefillRowsFromRecord(
        [{ key: "domains", type: "repeatable" }],
        opened,
      ).domains;
      await save(
        mapCompanyUpdate(
          values,
          rows,
          (current ?? []).map((domain) => ({
            domain: domain.domain ?? "",
            is_primary: domain.is_primary === "true",
          })),
        ),
      );
    },
    onSuccess: () => setOpen(false),
  });
  const shown = reading ?? t("field.unset");
  if (!canEdit) {
    return <span title={readOnlyReason}>{shown}</span>;
  }
  return (
    <>
      <Button
        variant="link"
        className="inline-editable"
        data-empty={!reading}
        aria-label={t("inlineChoice.change", { field: label })}
        onClick={() => {
          mutation.reset();
          setOpen(true);
        }}
      >
        {shown}
      </Button>
      <EditRecordModal
        open={open}
        onClose={() => setOpen(false)}
        title={label}
        fields={compoundFields(
          which,
          company,
          parent.name ?? undefined,
          t,
          locale,
        )}
        record={fieldRecord(company)}
        pending={mutation.isPending}
        error={mutation.error ? problemMessageOf(mutation.error, t) : null}
        onSubmit={(values, rows, opened) =>
          mutation.mutate({ values, rows: rows ?? {}, opened })
        }
      />
    </>
  );
}
