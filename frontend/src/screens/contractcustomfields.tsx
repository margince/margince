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
  values: Record<string, unknown>;
  onChange: (next: Record<string, unknown>) => void;
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
              stringValue(values[field.key]),
              (next) => onChange({ ...values, [field.key]: next }),
              t,
            )
          }
        </Field>
      ))}
    </>
  );
}

// The draft holds what the record holds — a number, a boolean, null — and the
// controls read and write strings. Null and undefined are the SAME empty here:
// a field nobody has answered and one somebody cleared both render blank.
function stringValue(raw: unknown): string {
  if (raw === null || raw === undefined) {
    return "";
  }
  return String(raw);
}
