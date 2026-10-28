// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { CreateField } from "../../components/create";
import { useEntityName } from "../../components/entityref";
import type { RailEdit } from "../../components/railfield";
import { saveRecordEdit } from "../../components/recordedit";
import type { FieldGroup } from "../../components/recordfieldtypes";
import { rawRecord } from "../../components/recordfieldtypes";
import { useOwnerPicker, useRecordOwners } from "../../components/recordowners";
import { useT } from "../../i18n";
import { useAcquisitionSources } from "../../lib/acquisitionsources.queries";
import { useCanWriteRecord } from "../../lib/capability";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import {
  NEW_PROJECT_OWNER,
  resolveDealProject,
  useProjectsOfCompany,
} from "./dealproject";
import type { Deal } from "./deals";
import {
  attributedPartner,
  dealEditFields,
  editProjectFields,
  usePartnerOptions,
} from "./deals.fields";
import { dealEditRecord, mapDealUpdate, stringValues } from "./deals.write";

// The masked money columns as the form's keys.
function formKey(column: string): string {
  if (column === "amount_minor") return "amount";
  if (column === "expected_arr_minor") return "expected_arr";
  return column;
}

/**
 * How a deal's fields change: the fields, the groups that change together,
 * the record they open on and the save. The details drawer and the left
 * column both edit through it.
 */
export function useDealEdit(
  deal: Deal,
  meId: string,
): RailEdit &
  Readonly<{ groups: FieldGroup[]; maskedFields: readonly string[] }> {
  const t = useT();
  const partnerOptions = usePartnerOptions([]);
  const acquisitionSources = useAcquisitionSources().data;
  const leadingCurrencies = useLeadingCurrencies();
  const masked = deal.masked_fields ?? [];
  const companyName = useEntityName("company", deal.company_id).name;
  const partnerName = useEntityName("company", deal.partner_company_id).name;
  const currentCompany = deal.company_id
    ? { id: deal.company_id, label: companyName ?? deal.company_id }
    : undefined;
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const formCompany = formValues.company_id ?? deal.company_id ?? undefined;
  const openProjects = useProjectsOfCompany(formCompany);
  const projectById = useEntityName("project", deal.project_id);
  const currentProject = deal.project_id
    ? { id: deal.project_id, label: projectById.name ?? deal.project_id }
    : undefined;
  const projectOwner = useOwnerPicker(null);
  const canEdit = useCanWriteRecord("deal", deal) && !deal.archived_at;
  const owners = useRecordOwners(deal.owner_id);
  // Company/project and money/currency are coupled writes: changing a visible
  // member must not silently rewrite a reference or unit the reader cannot see.
  const readOnlyFields: Record<string, string> = Object.fromEntries(
    masked.map((key) => [formKey(key), t("record.notShown")]),
  );
  if (deal.arr_source_offer_id)
    Object.assign(readOnlyFields, {
      expected_arr: t("deal.arrFromOffer"),
      currency: t("deal.arrFromOffer"),
    });
  const fields: CreateField[] = [
    ...dealEditFields(t, {
      partnerOptions,
      attributedPartner: attributedPartner(deal, partnerName),
      currentCompany,
      masked,
      me: meId,
      currentOwner: deal.owner_id ?? null,
      currency: deal.currency ?? "",
      leadingCurrencies,
      acquisitionSources,
      currentAcquisitionSource: deal.acquisition_source,
    }).map(
      (field): CreateField =>
        field.key === "owner_id"
          ? {
              ...field,
              label: "list.owner",
              options: owners.options,
              hint: owners.hint,
            }
          : field,
    ),
    ...editProjectFields(t, {
      masked,
      openProjects,
      currentProject:
        formCompany === deal.company_id ? currentProject : undefined,
      company: formCompany,
      owner: projectOwner,
    }),
  ];
  return {
    kind: "deal",
    canEdit,
    fields,
    readOnlyFields,
    maskedFields: masked
      .filter((key) =>
        ["amount_minor", "expected_arr_minor", "currency"].includes(key),
      )
      .map(formKey),
    groups: [
      {
        label: t(
          deal.arr_source_offer_id ? "deal.expectedArr" : "create.amount",
        ),
        keys: deal.arr_source_offer_id
          ? ["expected_arr", "currency"]
          : ["amount", "expected_arr", "currency"],
      },
      {
        label: t("create.relatedCompany"),
        keys: [
          "company_id",
          "project_id",
          "new_project_name",
          NEW_PROJECT_OWNER,
        ],
      },
      {
        label: t("deal.partnerCompany"),
        keys: ["partner_company_id", "partner_attribution"],
      },
    ],
    record: { ...dealEditRecord(deal), original: deal },
    onValuesChange: setFormValues,
    save: async (values, _rows, opened) => {
      const submitted = stringValues({ ...opened, ...values });
      const projectId = Object.hasOwn(values, "project_id")
        ? await resolveDealProject(
            submitted,
            submitted.company_id?.trim() || null,
            t,
          )
        : submitted.project_id;
      return saveRecordEdit(
        "deal",
        rawRecord(opened),
        mapDealUpdate(
          { ...opened, ...values, project_id: projectId ?? "" },
          opened,
          masked,
        ),
      );
    },
  };
}
