// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordOwnerOptions } from "../../components/assigneepicker";
import {
  RosterFailedLine,
  RosterPartialNote,
  useRosterPartial,
} from "../../components/entityref";
import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";

// Who each new lead from the form goes to. A saved owner the list does not
// hold stays chosen until changed. Leads for one who left arrive unassigned,
// and one a partial roster missed is simply kept.
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
  const unlisted =
    ownerId !== "" &&
    roster.isSuccess &&
    !options.some((option) => option.value === ownerId);
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
                      label: t(
                        partial
                          ? "webForms.field.ownerKept"
                          : "webForms.field.ownerGone",
                      ),
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
