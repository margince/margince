import { useCallback } from "react";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { useT } from "../i18n";
import {
  ADDRESS_FIELDS,
  addressFrom,
  companyEditFields,
  mapCompanyUpdate,
} from "./companyform";
import { joinMultiselectValue } from "./create";
import { prefillRowsFromRecord } from "./edit.prefill";
import { useEntityName } from "./entityref";
import { saveRecordEdit } from "./recordedit";
import { RecordFields, rawRecord } from "./recordfields";
import { searchCompanyReferences, useRecordOwners } from "./recordreferences";

type Company = components["schemas"]["Company"];
export function CompanyDetails({
  company,
}: Readonly<{
  company: Company;
}>) {
  const t = useT();
  const searchParents = useCallback(
    async (q: string) =>
      (await searchCompanyReferences(q)).filter(
        (candidate) => candidate.id !== company.id,
      ),
    [company.id],
  );
  const parent = useEntityName("company", company.parent_company_id);
  const owners = useRecordOwners(company.owner_id).map((entry) => ({
    id: entry.value,
    display_name: entry.label,
  }));
  return (
    <RecordFields
      kind="company"
      links={
        typeof company.linkedin_url === "string" && company.linkedin_url
          ? {
              linkedin_url: {
                href: company.linkedin_url,
                label: t("record.openProfile"),
              },
            }
          : undefined
      }
      canEdit={useCanWriteRecord("company", company) && !company.archived_at}
      groups={[
        {
          label: t("co.address.summary"),
          keys: ADDRESS_FIELDS.map((field) => field.key),
        },
      ]}
      title={t("co.details.title")}
      fields={[
        ...companyEditFields(owners, false, t),
        {
          key: "parent_company_id",
          label: "history.field.parent_company_id",
          searchTargets: searchParents,
          options: company.parent_company_id
            ? [
                {
                  value: company.parent_company_id,
                  label: parent.name ?? company.parent_company_id,
                },
              ]
            : [],
        },
        {
          key: "description",
          label: "co.description.label",
          type: "textarea",
          maxLength: 500,
        },
      ]}
      record={companyFieldRecord(company)}
      save={async (values, rows, opened) => {
        return saveRecordEdit(
          "company",
          rawRecord(opened),
          mapCompanyUpdate(
            values,
            rows,
            (
              prefillRowsFromRecord(
                [{ key: "domains", type: "repeatable" }],
                opened,
              ).domains ?? []
            ).map((domain) => ({
              domain: domain.domain,
              is_primary: domain.is_primary === "true",
            })),
          ),
        );
      }}
      resolveExisting={(_code, existingId) => ({
        screen: "companies",
        id: existingId,
      })}
    />
  );
}

function companyFieldRecord(company: Company) {
  return {
    original: company,
    id: company.id,
    version: company.version,
    display_name: company.display_name,
    parent_company_id: company.parent_company_id,
    owner_id: company.owner_id ?? "",
    legal_name: company.legal_name ?? "",
    industry: company.industry ?? "",
    size_band: company.size_band ?? "",
    lifecycle: company.lifecycle ?? "unknown",
    relationship_types: joinMultiselectValue(company.relationship_types ?? []),
    linkedin_url: company.linkedin_url ?? "",
    ...addressFrom(company.address),
    domains: (company.domains ?? []).map((domain) => ({
      domain: domain.domain,
      is_primary: String(domain.is_primary),
    })),
    description: company.description ?? "",
  };
}
