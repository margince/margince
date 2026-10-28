// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { INDUSTRY_FIELD, industryWords } from "../format/industry";
import { useT } from "../i18n";
import { historyFieldLabel } from "../lib/historyfieldlabels";

type MagicFieldChange = components["schemas"]["MagicFieldChange"];

/** Every field the change moved, old value to new. */
export function ChangeList({
  changes,
}: Readonly<{ changes: readonly MagicFieldChange[] }>) {
  const t = useT();
  if (changes.length === 0) {
    return null;
  }
  return (
    <dl className="magic-changes">
      {changes.map((change) => (
        <div key={change.field}>
          <dt className="t-caption">{historyFieldLabel(change.field, t)}</dt>
          <dd>
            {t("magic.records.fromTo", {
              from: valueText(change.field, change.before, t),
              to: valueText(change.field, change.after, t),
            })}
          </dd>
        </div>
      ))}
    </dl>
  );
}

// A stored value as text a reader recognises. Absent and empty read as "empty",
// a list as its items, anything structured as its JSON. Never cut: two long
// values that differ only at the end must still read as different.
function valueText(
  field: string,
  value: unknown,
  t: ReturnType<typeof useT>,
): string {
  if (value === null || value === undefined || value === "") {
    return t("magic.records.empty_value");
  }
  if (field === INDUSTRY_FIELD && typeof value === "string") {
    return industryWords(t, value);
  }
  if (Array.isArray(value)) {
    return value.map((item) => String(item)).join(", ");
  }
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}
