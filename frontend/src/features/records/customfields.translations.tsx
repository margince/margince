// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import type { CreateField } from "../../components/create";
import { EditAction } from "../../components/edit";
import {
  translationFields,
  translationsFromValues,
  translationValues,
} from "../../components/labeltranslations";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

type CustomField = components["schemas"]["CustomField"];

// The label's names travel under this prefix, and each option's under its
// position, so an option's own text never becomes a form key.
const LABEL = "label";
const optionPrefix = (index: number) => `option${index}`;

function translateFields(
  field: CustomField,
  t: ReturnType<typeof useT>,
): CreateField[] {
  const options = (field.options ?? []).flatMap((option, index) =>
    translationFields(optionPrefix(index)).map(
      (one): CreateField => ({
        ...one,
        label: undefined,
        labelText: t("cf.translateOption", {
          option,
          language: one.label ? t(one.label) : "",
        }),
      }),
    ),
  );
  return [...translationFields(LABEL), ...options];
}

function translateRecord(field: CustomField) {
  return {
    id: field.id,
    version: field.version,
    ...translationValues(LABEL, field.label_translations),
    ...Object.assign(
      {},
      ...(field.options ?? []).map((option, index) =>
        translationValues(
          optionPrefix(index),
          field.option_translations?.[option],
        ),
      ),
    ),
  };
}

function translateBody(field: CustomField, values: Record<string, unknown>) {
  return {
    label_translations: translationsFromValues(LABEL, values),
    option_translations: Object.fromEntries(
      (field.options ?? []).map((option, index) => [
        option,
        translationsFromValues(optionPrefix(index), values),
      ]),
    ),
  };
}

/**
 * The field's label and options in each UI language. A reader whose language
 * has a name sees it in forms and on records; the stored label and options
 * stay what records hold.
 */
export function CustomFieldTranslate({
  field,
}: Readonly<{ field: CustomField }>) {
  const t = useT();
  return (
    <EditAction<CustomField>
      labelled
      label={t("cf.translate")}
      savedMessage={t("cf.translated", { label: field.label })}
      invalidate="custom-fields"
      recordKey="custom_field"
      record={translateRecord(field)}
      fields={translateFields(field, t)}
      update={async (values, _rows, opened) =>
        unwrap(
          await api.PATCH("/custom-fields/{id}", {
            params: {
              path: { id: field.id },
              ...ifMatch(requireVersion(opened?.version)),
            },
            body: translateBody(field, values),
          }),
        )
      }
    />
  );
}
