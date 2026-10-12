// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useTagVocabulary } from "./tags.queries";

/**
 * What the approver is committing to, named rather than implied.
 *
 * The picker lives on the mapping step, which is off screen once a report
 * exists — so the run's word would otherwise be invisible at the one moment
 * somebody decides whether to write it onto every created record.
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
          // "a tag" is honest; naming the wrong one, or none, is not.
          t("import.contextTagChosenUnnamed")}
    </p>
  );
}

/**
 * The word every record this run CREATES is filed under.
 *
 * Optional, and an existing word only: an import that coined one would hand the
 * vocabulary's one governed door to anyone who can upload a file, and a
 * misspelled column header would become a permanent tag nobody chose.
 *
 * Creates only, which the label says. A row that UPDATES a record the estate
 * already held leaves its tags alone — the run did not put it there, and
 * tagging it would claim the batch contains records it only touched.
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
        // The catalog is capped and carries no cursor, so past the cap a word
        // that exists cannot be picked — and an importer who cannot find the
        // word they meant would file the batch under nothing, or ask an admin
        // to coin a duplicate.
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
