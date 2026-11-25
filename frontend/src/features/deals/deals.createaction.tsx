// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { api } from "../../api/client";
import { searchCompanyTargets } from "../../components/companyform";
import { CreateAction } from "../../components/create";
import { amountField, currencyField } from "../../components/currencyfield";
import type { ObjectCustomFields } from "../../components/customfields.form";
import { useDealCreateFields } from "../../components/dealcreatefields";
import { useOwnerPicker } from "../../components/recordowners";
import {
  type ListRefusal,
  useListRefusal,
} from "../../design-system/listtable";
import { useLocale, useT } from "../../i18n";
import { mapDealCreate } from "../../lib/dealcreate";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import { throwProblem, unwrap } from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { useActionRefusal } from "../../lib/writerefusal";
import {
  dealProjectFields,
  resolveDealProject,
  useProjectsOfCompany,
} from "./dealproject";
import type { Pipeline, Stage } from "./deals";

// The create form, with the one question it has to ask the server: which
// projects the company named in the open form is on.
//
// Its own component because that question is the form's and not the screen's.
// The company comes from the form's own values. So the state that holds it and
// the query that follows it belong beside the form, not among the screen's dials.
export function DealCreateAction({
  pipeline,
  cf,
  openStages,
  partnerOptions,
  startOpen,
  refusalId,
}: Readonly<{
  pipeline?: Pipeline;
  // Read at screen level and handed down, so the schema request runs beside the
  // screen's own. It does not wait for the pipelines this form is gated on.
  cf: ObjectCustomFields;
  openStages: Stage[];
  partnerOptions: { value: string; label: string }[];
  startOpen: boolean;
  // The list's sentence saying why a refused create is refused.
  refusalId?: string;
}>) {
  const t = useT();
  const leading = useLeadingCurrencies();
  const { locale } = useLocale();
  // The picker asks the server for one company's projects, so it is keyed on
  // the company the open form names. A create form has no record to take it
  // from, and only the server knows which projects a company is on. The form
  // publishes its answers (CreateAction's onValuesChange) and the query follows.
  const [formCompany, setFormCompany] = useState("");
  const openProjects = useProjectsOfCompany(formCompany || undefined);
  const projectOwner = useOwnerPicker(null);
  const optionalFields = useDealCreateFields(t, projectOwner, partnerOptions);

  const createDeal = async (
    values: Record<string, string>,
    _rows: unknown,
    idempotencyKey: string,
  ) => {
    if (!pipeline) {
      throwProblem(null);
    }
    // A project asked for on the form is born first, so the deal names it.
    const projectId = await resolveDealProject(
      values,
      values.company_id?.trim() || null,
      t,
      idempotencyKey,
    );
    return unwrap(
      await api.POST("/deals", {
        params: { header: { "Idempotency-Key": idempotencyKey } },
        body: {
          ...mapDealCreate(
            { ...values, project_id: projectId ?? "" },
            pipeline.id,
            leading[0],
            locale,
          ),
          ...cf.toBody(values),
        },
      }),
      t,
    );
  };

  return (
    <CreateAction
      label={t("create.deal")}
      requires={{ object: "deal", action: "create" }}
      refusalId={refusalId}
      invalidate="deals"
      screen="deals"
      create={createDeal}
      listCreate={{ open: startOpen }}
      onValuesChange={(values) => setFormCompany(values.company_id ?? "")}
      fields={[
        {
          key: "name",
          label: "create.dealName",
          required: true,
          maxLength: TEXT_LIMITS.label,
        },
        amountField(
          { key: "amount", label: "create.amount", wireField: "amount_minor" },
          {
            currency: (values) => values.currency || leading[0],
            locale,
            t,
            nonNegative: true,
          },
        ),
        currencyField(leading, locale),
        {
          key: "stage_id",
          label: "create.stage",
          type: "select",
          required: true,
          options: openStages.map((stage) => ({
            value: stage.id,
            label: stage.name,
          })),
        },
        // Searched: the screen's own page of companies names board cards and
        // stops short of a large workspace.
        {
          key: "company_id",
          label: "create.relatedCompany",
          searchTargets: searchCompanyTargets,
        },
        // The body of work this deal is about, chosen or started here: a
        // project begins during the deal, in its initiative phase.
        // Narrowed by the server, for the company the form names now. The
        // query above re-reads when the reader changes it, so the picker
        // repopulates instead of going empty.
        ...dealProjectFields(
          t,
          openProjects,
          undefined,
          formCompany,
          projectOwner,
        ),
        ...optionalFields,
        {
          key: "expected_close_date",
          label: "create.expectedClose",
          type: "date",
        },
        ...cf.formFields,
      ]}
    />
  );
}

// The list's sentence for a refused create, while a create is offered at all.
export function useCreateRefusal(offered: boolean): ListRefusal | undefined {
  const refusal = useListRefusal(
    useActionRefusal({ object: "deal", action: "create" }),
  );
  return offered ? refusal : undefined;
}
