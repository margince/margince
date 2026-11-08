// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatMoney } from "../format/format";
import { formatDate, formatDateTime } from "../format/format.dates";
import { INDUSTRY_FIELD, industryWords } from "../format/industry";
import { type Locale, useLocale, useT } from "../i18n";
import { historyFieldLabel } from "../lib/historyfieldlabels";
import { useViewerZone } from "../lib/viewerzone";

type MagicFieldChange = components["schemas"]["MagicFieldChange"];

/** Every field the change moved, old value to new. */
export function ChangeList({
  changes,
}: Readonly<{ changes: readonly MagicFieldChange[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  if (changes.length === 0) {
    return null;
  }
  const format = { t, locale, zone };
  return (
    <dl className="magic-changes">
      {changes.map((change) => (
        <div key={change.field}>
          <dt className="t-caption">{historyFieldLabel(change.field, t)}</dt>
          <dd>
            {t("magic.records.fromTo", {
              from: valueText(
                change,
                change.before,
                change.before_label,
                format,
              ),
              to: valueText(change, change.after, change.after_label, format),
            })}
          </dd>
        </div>
      ))}
    </dl>
  );
}

// A stored value as text a reader recognises. The server says how to read
// it: money with its currency, a calendar day or an instant, a stage or a
// member by the name the reader may see. A value it cannot name is never shown
// as its raw id, minor units or code. Absent and empty read as "empty"; a
// list as its items. Never cut: two long values that differ only at the end
// must still read as different.
function valueText(
  change: MagicFieldChange,
  value: unknown,
  label: string | undefined,
  format: Formatting,
): string {
  const { t } = format;
  if (value === null || value === undefined || value === "") {
    return t("magic.records.empty_value");
  }
  switch (change.type) {
    case "money":
      return moneyText(change, value, format);
    case "date":
      return dateText(change, value, format);
    case "stage":
    case "member":
    case "reference":
      return label ?? t("magic.records.hidden_value");
    case "boolean":
      return t(value === true ? "field.yes" : "field.no");
    case "enum":
      return change.field === INDUSTRY_FIELD && typeof value === "string"
        ? industryWords(t, value)
        : t("magic.records.hidden_value");
    case "number":
    case "text":
      return String(value);
  }
  return untypedText(change.field, value, t);
}

function moneyText(
  change: MagicFieldChange,
  value: unknown,
  { t, locale }: Formatting,
): string {
  return typeof value === "number" && change.currency
    ? formatMoney(value, change.currency, locale)
    : t("magic.records.hidden_value");
}

function dateText(
  change: MagicFieldChange,
  value: unknown,
  { t, locale, zone }: Formatting,
): string {
  if (typeof value !== "string") return t("magic.records.hidden_value");
  return change.precision === "instant"
    ? formatDateTime(value, locale, zone)
    : formatDate(value, locale, zone);
}

// An older server types nothing. Only a value no field-specific reading can
// get wrong is printed: never minor units, an id or a structure.
function untypedText(
  field: string,
  value: unknown,
  t: ReturnType<typeof useT>,
): string {
  if (field === INDUSTRY_FIELD && typeof value === "string") {
    return industryWords(t, value);
  }
  if (Array.isArray(value) && value.every((item) => typeof item === "string")) {
    return value.join(", ");
  }
  if (
    field.endsWith("_minor") ||
    field.endsWith("_id") ||
    typeof value === "object" ||
    (typeof value === "string" && UUID.test(value))
  ) {
    return t("magic.records.hidden_value");
  }
  return String(value);
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

type Formatting = Readonly<{
  t: ReturnType<typeof useT>;
  locale: Locale;
  zone: string;
}>;
