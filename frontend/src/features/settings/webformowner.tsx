// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useSeatedOwnerOptions } from "../../components/assigneepicker";
import {
  EntityRef,
  RosterFailedLine,
  RosterPartialNote,
  useRosterPartial,
} from "../../components/entityref";
import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useDepartedName } from "../../lib/membernames";

// Who each new lead from the form goes to. Any colleague may be named, since
// the installation writes the lead, not the one who set up the form. A saved
// owner the list does not hold stays chosen; one the server names as departed
// is shown as such, since their leads arrive unassigned.
export function OwnerField({
  ownerId,
  error,
  onChange,
}: Readonly<{
  ownerId: string;
  error?: string;
  onChange: (ownerId: string) => void;
}>) {
  const t = useT();
  const { options, roster } = useSeatedOwnerOptions();
  const partial = useRosterPartial("user", true);
  const departed = useDepartedName(ownerId || null);
  const unlisted =
    ownerId !== "" && !options.some((option) => option.value === ownerId);
  return (
    <div className="form-stack webform-part">
      <Field
        label={t("webForms.field.owner")}
        hint={t("webForms.field.ownerHint")}
        error={error}
      >
        {(field) => (
          <Select
            {...field}
            options={[
              { value: "", label: t("webForms.field.ownerNone") },
              ...(unlisted
                ? [
                    {
                      value: ownerId,
                      label: departed
                        ? t("webForms.field.ownerGone", { name: departed })
                        : t("webForms.field.ownerKept"),
                    },
                  ]
                : []),
              ...options,
            ]}
            value={ownerId}
            onChange={onChange}
          />
        )}
      </Field>
      <RosterFailedLine kind="user" roster={roster} />
      <RosterPartialNote partial={partial} />
    </div>
  );
}

// The row's word on where the form's leads go, which stops naming an owner
// who left as their destination.
export function FormOwnerLine({ ownerId }: Readonly<{ ownerId: string }>) {
  const t = useT();
  const departed = useDepartedName(ownerId);
  if (departed) {
    return <span>{t("webForms.row.ownerLeft", { name: departed })}</span>;
  }
  return (
    <span>
      {t("webForms.row.owner")} <EntityRef kind="user" id={ownerId} asText />
    </span>
  );
}
