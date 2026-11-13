// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import { PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { labelFor } from "../../lib/labelfor";
import { AddFactForm, useCompanyFacts } from "./companyfactspanel";
import {
  useCompanyEditGate,
  useIndustryOptions,
  useRowBuilders,
} from "./companyrail";
import { SidecarFieldValue } from "./companyraildetails";
import { useCompanyProfileFields } from "./evidenceverdict";

type Company = components["schemas"]["Company"];
type DataCompleteness = components["schemas"]["DataCompleteness"];
type GrowthFitInput = components["schemas"]["GrowthFitInput"];
type ProfileFieldKey = components["parameters"]["ProfileFieldKey"];
type Edit = ReturnType<typeof useCompanyEditGate>;

// Each row reads as the Profile tab and facts panel name the same field. A
// full record, so an input added to the contract without a label fails tsc.
const ROW_LABELS: Record<GrowthFitInput, MessageKey> = {
  offer_summary: "co.profileField.offer_summary",
  icp: "co.profileField.icp",
  industry: "co.profileField.industry",
  buying_center: "co.profileField.buying_center",
  buying_intents: "co.profileField.buying_intents",
  employee_range: "co.factField.employee_range",
  technology: "co.factField.technology",
};

// The inputs held as profile fields, written through the Profile tab's writer.
// Industry, size and technology live elsewhere and have rows of their own.
const PROFILE_INPUTS: Partial<Record<GrowthFitInput, ProfileFieldKey>> = {
  offer_summary: "offer_summary",
  icp: "icp",
  buying_center: "buying_center",
  buying_intents: "buying_intents",
};

/**
 * GrowthFitInputs is what the fit is judged on: each input with its value or
 * gap, and the rule for judging. A gap is filled in place with the Profile
 * tab's own editors, so a value added here is the same write as there.
 */
export function GrowthFitInputs({
  company,
  completeness,
}: Readonly<{ company: Company; completeness: DataCompleteness }>) {
  const t = useT();
  const { locale } = useLocale();
  const reasonId = useId();
  const edit = useCompanyEditGate(company);
  return (
    <PanelBody>
      <Eyebrow as="h3">{t("co.growthFit.inputs")}</Eyebrow>
      <p className="t-caption">
        {t("co.growthFit.rule", {
          needed: formatNumber(completeness.needed, locale),
          expected: formatNumber(completeness.expected, locale),
        })}
      </p>
      {edit.reason && (
        <p id={reasonId} className="t-caption">
          {edit.reason}
        </p>
      )}
      <FieldGrid>
        {completeness.inputs.map((state) => (
          <InputRow
            key={`${company.id}:${state.input}`}
            company={company}
            input={state.input}
            present={state.present}
            edit={edit}
            reasonId={edit.reason ? reasonId : undefined}
          />
        ))}
      </FieldGrid>
    </PanelBody>
  );
}

type RowProps = Readonly<{
  company: Company;
  input: GrowthFitInput;
  present: boolean;
  edit: Edit;
  reasonId: string | undefined;
}>;

function InputRow(props: RowProps) {
  const field = PROFILE_INPUTS[props.input];
  if (field) {
    return <ProfileInputRow {...props} field={field} />;
  }
  if (props.input === "industry") {
    return <IndustryRow {...props} />;
  }
  return <FactInputRow {...props} />;
}

function ProfileInputRow({
  company,
  input,
  present,
  edit,
  field,
}: RowProps & { field: ProfileFieldKey }) {
  const t = useT();
  const profile = useCompanyProfileFields(company.id);
  const fields = profile.data ?? [];
  const recorded = Boolean(
    fields.find((one) => one.field === field)?.value.trim(),
  );
  return (
    <FieldRow label={t(labelFor(ROW_LABELS, input))}>
      <InputState present={present} recorded={recorded} />
      <SidecarFieldValue
        companyId={company.id}
        anchor={company.is_anchor === true}
        fields={fields}
        fieldsLoaded={profile.isSuccess}
        field={field}
        label={t(labelFor(ROW_LABELS, input))}
        placeholderKey="co.narrative.add"
        canEdit={edit.canEdit}
        readOnlyReason={edit.reason}
        multiline
      />
    </FieldRow>
  );
}

// The fit counts a site-read industry when the record's own is empty. So the
// row shows that read value too.
function IndustryRow({ company, input, present, edit }: RowProps) {
  const t = useT();
  const { choice } = useRowBuilders(company, edit);
  const options = useIndustryOptions(company);
  const row = choice("industry", "create.industry", options);
  const profile = useCompanyProfileFields(company.id);
  const read = row.empty
    ? (profile.data ?? []).find((one) => one.field === "industry")?.value.trim()
    : undefined;
  return (
    <FieldRow label={t(labelFor(ROW_LABELS, input))}>
      <InputState present={present} recorded={!row.empty || Boolean(read)} />
      {read && <span>{read} </span>}
      {row.value}
    </FieldRow>
  );
}

// Size and technology are facts. Size is one a contact may state; technology
// comes only from reading the site, so its gap names that instead of a form.
const STATEABLE_FACTS: Partial<Record<GrowthFitInput, string>> = {
  employee_range: "company:employee_range",
};

function FactInputRow({ company, input, present, edit, reasonId }: RowProps) {
  const t = useT();
  const [adding, setAdding] = useState(false);
  const facts = useCompanyFacts(company.id).data ?? [];
  const values = facts
    .filter((fact) => fact.field === input && fact.value.trim())
    .map((fact) => fact.value);
  const stateable = STATEABLE_FACTS[input];
  let action: ReactNode = null;
  if (stateable && adding) {
    action = (
      <AddFactForm
        companyId={company.id}
        anchor={company.is_anchor === true}
        canEdit={edit.canEdit}
        only={stateable}
        onDone={() => setAdding(false)}
      />
    );
  } else if (stateable && !present) {
    action = (
      <Button
        variant="link"
        onClick={() => setAdding(true)}
        reasonId={edit.canEdit ? undefined : reasonId}
        unavailable={!edit.canEdit}
      >
        {t("co.narrative.add")}
      </Button>
    );
  } else if (!present) {
    action = (
      <span className="t-caption">{t("co.growthFit.fromSiteRead")}</span>
    );
  }
  return (
    <FieldRow label={t(labelFor(ROW_LABELS, input))}>
      <InputState present={present} recorded={values.length > 0} />
      {values.length > 0 && <span>{values.join(", ")} </span>}
      {action}
    </FieldRow>
  );
}

// An input that does not count is absent, or recorded and stale. The two ask
// different things of the reader: add it, or check it again.
function InputState({
  present,
  recorded,
}: Readonly<{ present: boolean; recorded: boolean }>) {
  const t = useT();
  if (present) {
    return null;
  }
  return (
    <>
      <Badge tone="warning">
        {t(recorded ? "co.growthFit.inputStale" : "co.growthFit.inputMissing")}
      </Badge>{" "}
    </>
  );
}
