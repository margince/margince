// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { RailEdit } from "../../components/railfield";
import { saveRecordEdit } from "../../components/recordedit";
import { type FieldRecord, rawRecord } from "../../components/recordfieldtypes";
import { useT } from "../../i18n";
import { useIndustryOptions } from "../../lib/industryoptions";
import { leadWriteKeys } from "../../lib/leadkeys";
import { originPickOptions, useLeadOrigins } from "../../lib/leadorigins";
import { ProblemError } from "../../lib/problem";
import { useLeadPartnerOptions } from "./leadpartner";
import type { Lead, LeadWriter } from "./leads";
import { leadEditFields, mapLeadUpdate } from "./leads.form";

/** The one write behind a lead's own fields: its details sections and its head's rename. */
function saveLeadFields(opened: FieldRecord, values: Record<string, string>) {
  return saveRecordEdit("lead", rawRecord(opened), mapLeadUpdate(values));
}

/**
 * How a lead's fields change: the fields, the record they open on and the
 * save. Each details section edits the part of it that it shows.
 */
export function useLeadEdit(
  lead: Lead,
  writer: Pick<LeadWriter, "readOnly">,
): RailEdit {
  const t = useT();
  const origins = useLeadOrigins();
  const partners = useLeadPartnerOptions() ?? [];
  const industries = useIndustryOptions(lead.industry);
  const fields = leadEditFields({
    industries: [{ value: "", label: t("field.unset") }, ...industries],
    // "Not set" is a real choice: choosing it sends null and the lead scores neutral.
    origins: [
      { value: "", label: t("lead.originNone") },
      ...originPickOptions(origins.data, lead.origin),
    ],
    partners: [{ value: "", label: t("lead.partnerNone") }, ...partners],
  });
  return {
    kind: "lead",
    record: lead,
    canEdit: !writer.readOnly,
    // A blank name is not required here, or it would hold every other change hostage.
    fields: fields.map((field) =>
      field.key === "full_name"
        ? { ...field, required: Boolean(lead.full_name) }
        : field,
    ),
    readOnlyFields: {
      linkedin_url: t("record.leadProfileReadOnly"),
      // A linked lead's company name is the company's own.
      ...(lead.company_id
        ? { company_name: t("lead.company.nameFollowsLink") }
        : {}),
    },
    save: (values, _rows, opened) => saveLeadFields(opened, values),
  };
}

/** The head's rename: the lead's name through the same write as its details. */
export function useLeadRenameSave(lead: Lead) {
  const t = useT();
  const queryClient = useQueryClient();
  return async (next: string) => {
    if (!next.trim()) {
      throw new ProblemError({ detail: t("record.fieldRequired") });
    }
    await saveLeadFields(lead, { full_name: next });
    for (const queryKey of leadWriteKeys(lead.id)) {
      await queryClient.invalidateQueries({ queryKey });
    }
  };
}
