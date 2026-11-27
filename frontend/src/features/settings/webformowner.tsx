// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordOwnerOptions } from "../../components/assigneepicker";
import {
  OwnerName,
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
  const { options, roster } = useRecordOwnerOptions();
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

// Where the form's new leads go. An inquiry-handling policy decides that for
// itself, and an owner who left is named as gone rather than as a destination.
export function FormOwnerCell({
  ownerId,
  handled,
}: Readonly<{ ownerId: string | null | undefined; handled: boolean }>) {
  const t = useT();
  const departed = useDepartedName(handled ? null : (ownerId ?? null));
  if (handled) {
    return <span>{t("inquiry.handling")}</span>;
  }
  if (departed) {
    return <span>{t("webForms.row.ownerLeft", { name: departed })}</span>;
  }
  return (
    <OwnerName ownerId={ownerId} unowned={t("webForms.field.ownerNone")} face />
  );
}
