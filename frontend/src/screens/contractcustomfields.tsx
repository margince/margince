// The workspace's own fields on the contract form.
//
// A separate file because contractform.tsx is at its length ceiling, and a
// separate COMPONENT because the named terms beside it are shared with the
// renewal modal: a renewal inherits nothing, so custom values must not ride
// inside ContractTermsFields where they would follow the terms across.
//
// The contract form is a plain modal over a typed draft rather than the
// CreateAction/EditAction harness every other custom-field surface uses, so
// this renders the same controls that harness does — fieldControl and
// fieldLabel, not a second set — and converts at the boundary: the draft holds
// stored values, the controls speak strings.

import { Field } from "../design-system/atoms";
import { useT } from "../i18n";
import { type CreateField, fieldControl, fieldLabel } from "./create";

export function ContractCustomFields({
  fields,
  values,
  onChange,
}: Readonly<{
  fields: CreateField[];
  // FORM strings, not stored values. Every converter in customfields.form.ts
  // reads and writes what the control holds — a currency field's major units,
  // a boolean's "true" — and the stored shape is derived from that afterwards.
  values: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
}>) {
  const t = useT();
  // The divider the catalog injects names the section on forms that already
  // have one. This form has no other sections, so the heading would be a lone
  // label over the only group on screen.
  const shown = fields.filter((field) => !field.divider);
  if (shown.length === 0) {
    return null;
  }
  return (
    <>
      {shown.map((field) => (
        <Field key={field.key} label={fieldLabel(field, t)} hint={field.hint}>
          {(control) =>
            fieldControl(
              field,
              control,
              values[field.key] ?? "",
              (next) => onChange({ ...values, [field.key]: next }),
              t,
            )
          }
        </Field>
      ))}
    </>
  );
}
