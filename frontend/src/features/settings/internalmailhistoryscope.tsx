// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Checkbox } from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { Field } from "../../design-system/field";
import { useT } from "../../i18n";

export function InternalMailHistoryScope({
  value,
  disabled,
  onChange,
}: Readonly<{
  value: string | undefined;
  disabled: boolean;
  onChange: (value: string | undefined) => void;
}>) {
  const t = useT();
  const selected = value !== undefined;
  return (
    <div className="form-stack">
      <Checkbox
        label={t("internalMail.authorizeHistory")}
        checked={selected}
        disabled={disabled}
        onChange={() => onChange(selected ? undefined : "")}
      />
      {selected && (
        <Field
          label={t("internalMail.historySince")}
          hint={t("internalMail.historyApprovalNote")}
        >
          {(control) => (
            <DateInput
              {...control}
              value={isoOrBlank(value)}
              disabled={disabled}
              onChange={(event) => onChange(event.target.value)}
            />
          )}
        </Field>
      )}
    </div>
  );
}
