// The workspace's activity custom fields on the Log activity form, kept apart
// from logactivity.tsx, which sits at its line ceiling.

import { type ReactNode, useState } from "react";
import { CustomFieldControls } from "./customfieldcontrols";
import { useObjectCustomFields } from "./customfields.form";

export type ActivityCustomFields = Readonly<{
  controls: ReactNode;
  /** The values as the create body's cf_ keys. */
  body: () => Record<string, unknown>;
  reset: () => void;
}>;

/**
 * The activity fields an admin keeps on create forms and applies to `kind`.
 * A kind change drops the value of a field the new kind does not take. It is
 * not sent, and a switch back does not restore it.
 */
export function useActivityCustomFields(kind: string): ActivityCustomFields {
  const catalog = useObjectCustomFields("activity", "create", { kind });
  const [values, setValues] = useState<Record<string, string>>({});
  const [valuesKind, setValuesKind] = useState(kind);
  if (kind !== valuesKind) {
    setValuesKind(kind);
    setValues((current) => {
      const kept: Record<string, string> = {};
      for (const field of catalog.fields) {
        const typed = current[field.column_name];
        if (typed !== undefined) kept[field.column_name] = typed;
      }
      return kept;
    });
  }
  return {
    controls: (
      <CustomFieldControls
        fields={catalog.formFields}
        values={values}
        onChange={setValues}
      />
    ),
    body: () => catalog.toBody(values),
    reset: () => setValues({}),
  };
}
