// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { ContactCase } from "./contactcase.queries";
import {
  type CasePreference,
  type PreferField,
  preferChoices,
} from "./contactcase.selection";

const LABELS: Readonly<Record<PreferField, MessageKey>> = {
  name: "contactCase.prefer.name",
  email: "contactCase.prefer.email",
  employment: "contactCase.prefer.employment",
  title: "contactCase.prefer.title",
};

type Props = Readonly<{
  contactCase: ContactCase;
  selected: readonly string[];
  survivor: string;
  prefer: CasePreference;
  disabled: boolean;
  onChange: (field: PreferField, member: string) => void;
}>;

/** One choice per field the ticked members disagree on. */
export function CasePreferences({
  contactCase,
  selected,
  survivor,
  prefer,
  disabled,
  onChange,
}: Props) {
  const t = useT();
  const choices = preferChoices(contactCase, selected, survivor);
  if (choices.length === 0) return null;
  return (
    <fieldset className="form-stack">
      <legend>{t("contactCase.prefer.legend")}</legend>
      <p className="t-caption">{t("contactCase.prefer.hint")}</p>
      {choices.map(({ field, options }) => {
        const kept = options.find((option) => option.member === survivor);
        const chosen = options.find(
          (option) => option.member === prefer[field],
        );
        return (
          <Field key={field} label={t(LABELS[field])}>
            {(control) => (
              <Select
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                options={options.map((option) => ({
                  value: option.member,
                  label: option.value,
                }))}
                value={(chosen ?? kept)?.member ?? ""}
                placeholder={t("contactCase.prefer.placeholder")}
                disabled={disabled}
                onChange={(next) => onChange(field, next)}
              />
            )}
          </Field>
        );
      })}
    </fieldset>
  );
}
