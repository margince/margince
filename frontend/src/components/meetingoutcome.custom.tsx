// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The meeting's custom fields in the "Add notes" dialog, kept apart from
// meetingoutcome.tsx, which sits near its line ceiling.

import { type ReactNode, useState } from "react";
import type { components } from "../api/schema";
import { CustomFieldControls } from "./customfieldcontrols";
import {
  customFieldFormValue,
  useObjectCustomFields,
} from "./customfields.form";

type Activity = components["schemas"]["Activity"];

export type MeetingCustomFields = Readonly<{
  controls: ReactNode;
  /** The edited values only, as the patch's cf_ keys; a cleared one is null. */
  patch: () => Record<string, unknown>;
}>;

const NO_ROW: Record<string, unknown> = {};

/**
 * The fields that apply to the meeting, prefilled from it. A field that no
 * longer applies shows too while it holds a value, so the reader can clear it.
 */
export function useMeetingCustomFields(
  meeting: Activity | undefined,
): MeetingCustomFields {
  const held: Record<string, unknown> = meeting ?? NO_ROW;
  const catalog = useObjectCustomFields("activity", "record", {
    kind: meeting?.kind ?? "meeting",
    held,
  });
  const [values, setValues] = useState<Record<string, string> | null>(null);
  // Seeded during render once both reads answer, as the dialog seeds its draft.
  if (values === null && meeting && !catalog.loading) {
    const seeded: Record<string, string> = {};
    for (const field of catalog.fields) {
      seeded[field.column_name] = customFieldFormValue(
        field,
        held[field.column_name],
      );
    }
    setValues(seeded);
  }
  return {
    controls: values && (
      <CustomFieldControls
        fields={catalog.formFields}
        values={values}
        onChange={setValues}
      />
    ),
    patch: () =>
      values ? catalog.toPatch(values, catalog.recordSlice(held)) : {},
  };
}
