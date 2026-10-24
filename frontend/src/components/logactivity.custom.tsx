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
 * The fields an admin added to activities and keeps on create forms. They apply
 * to every kind, so the form offers them whatever is being logged.
 */
export function useActivityCustomFields(): ActivityCustomFields {
  const catalog = useObjectCustomFields("activity", "create");
  const [values, setValues] = useState<Record<string, string>>({});
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
