// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useTagVocabulary } from "../../lib/tags.queries";

/**
 * What the approver is committing to, named rather than implied.
 *
 * The picker lives on the mapping step, off screen once a report exists. So
 * the run's word would be hidden when somebody decides to write it onto every
 * created record.
 */
export function ImportContextTagSummary({
  tagID,
}: Readonly<{ tagID: string }>) {
  const t = useT();
  const vocabulary = useTagVocabulary();
  if (tagID === "") {
    return null;
  }
  const word = vocabulary.data?.tags.find((tag) => tag.id === tagID);
  return (
    <p className="import__hint t-sub">
      {word
        ? t("import.contextTagChosen", { name: word.name })
        : // The vocabulary has not landed, or no longer holds the word. Saying
          // "a tag" is true; naming the wrong one, or none, is not.
          t("import.contextTagChosenUnnamed")}
    </p>
  );
}

/**
 * The word every record this run creates is filed under. Existing words only:
 * coining one would open the vocabulary to anyone who can upload a file.
 * A misspelled header would then become a tag nobody chose.
 * An updated row keeps its tags, since the run only touched that record.
 */
export function ImportContextTag({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (next: string) => void }>) {
  const t = useT();
  const vocabulary = useTagVocabulary();
  const words = vocabulary.data?.tags ?? [];
  if (words.length === 0) {
    // No vocabulary, or none this caller may read. A dial whose only option is
    // "none" asks a question with one answer.
    return null;
  }
  return (
    <Field
      label={t("import.contextTag")}
      hint={
        // The catalog is capped with no cursor, so past the cap a word cannot be
        // picked. An importer who cannot find it would file the batch under
        // nothing, or ask an admin to coin a duplicate.
        vocabulary.data?.truncated
          ? `${t("import.contextTagHint")} ${t("tags.catalogTruncated")}`
          : t("import.contextTagHint")
      }
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          onChange={onChange}
          options={[
            { value: "", label: t("import.contextTagNone") },
            ...words.map((tag) => ({ value: tag.id, label: tag.name })),
          ]}
        />
      )}
    </Field>
  );
}
