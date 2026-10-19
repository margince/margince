// The workspace's own fields on a plain modal over a typed draft, such as the
// contract form and the Log activity form. The CreateAction harness is not used.
//
// It renders the harness's own controls (fieldControl and fieldLabel). The draft
// holds form strings, and the caller converts them with the catalog's toBody.

import { Field } from "../design-system/atoms";
import { useT } from "../i18n";
import { type CreateField, fieldControl, fieldLabel } from "./create";

export function CustomFieldControls({
  fields,
  values,
  onChange,
}: Readonly<{
  fields: CreateField[];
  // Form strings, not stored values: a currency field's major units, a
  // boolean's "true". The stored shape is derived from them afterwards.
  values: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
}>) {
  const t = useT();
  // The divider the catalog injects names a section, and these forms draw the
  // fields as one more group with no heading of its own.
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
