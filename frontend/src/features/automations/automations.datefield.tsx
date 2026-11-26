// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { AppLink } from "../../design-system/applink";
import type { FieldControl } from "../../design-system/atoms";
import type { SelectOption } from "../../design-system/select";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { customFieldsQuery } from "../../lib/customfields.query";
import { routeHash } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import "./automations.datefield.css";

// renewal_reminder's date_field param (GH-706) names a workspace's own cf_*
// date column — a free-text box would let an operator type a column that
// does not exist, or exists but isn't a date, and the save-time validator
// deliberately does not catch that (automations_catalog.go's
// validateRenewalReminderParams checks only non-emptiness; the real
// existence/type check runs at scan time, per DESIGN.md). This picker
// closes that gap client-side by offering only real, active, date-typed
// columns for whichever object the form's own `object` param currently
// holds — the same /custom-fields list the custom-fields settings screen
// already renders (customfields.tsx), scoped and filtered here rather than
// server-side, since the endpoint's own filter vocabulary is object + status
// only (no field-type filter).

// An `object` outside this set never asks /custom-fields: the picker shows its
// disabled, hinted state instead.
const CUSTOM_FIELD_QUERY_OBJECTS = new Set([
  "contact",
  "company",
  "deal",
  "lead",
  "project",
]);

type CustomFieldQueryObject =
  | "contact"
  | "company"
  | "deal"
  | "lead"
  | "project";

function isCustomFieldQueryObject(
  value: string,
): value is CustomFieldQueryObject {
  return CUSTOM_FIELD_QUERY_OBJECTS.has(value);
}

const RECORDS_SETTINGS_HREF = routeHash(settingsHref("records"));

function WhereDateFieldsAreAdded() {
  const t = useT();
  const [before, after = ""] = t("auto.dateField.empty").split("{place}");
  return (
    <>
      {before}
      <AppLink href={RECORDS_SETTINGS_HREF}>
        {t("auto.dateField.place")}
      </AppLink>
      {after}
    </>
  );
}

export function DateFieldSelect({
  object,
  value,
  onChange,
  control,
}: Readonly<{
  object: string;
  value: string;
  onChange: (value: string) => void;
  /** What the enclosing `Field` hands its control — the id its label points
   *  at, and the description wiring that goes with it. */
  control: FieldControl;
}>) {
  const t = useT();
  const known = isCustomFieldQueryObject(object);
  const fields = useQuery(customFieldsQuery(known ? object : undefined));

  const options: SelectOption[] = known
    ? (fields.data?.data ?? [])
        .filter((field) => field.status === "active" && field.type === "date")
        .map((field) => ({ value: field.column_name, label: field.label }))
    : [];

  // Disabled whenever there is nothing real to choose from yet — no object
  // picked, the fetch itself failed, or the chosen object simply has no
  // active date field — rather than opening on an empty list a click
  // could never fill, or (worse) an empty list with no indication the
  // fetch behind it failed.
  const disabled =
    !known || fields.isError || (fields.isSuccess && options.length === 0);
  let hint: ReactNode;
  if (!known) {
    hint = t("auto.dateField.needsObject");
  } else if (fields.isError) {
    hint = t("auto.dateField.loadError");
  } else if (fields.isSuccess && options.length === 0) {
    hint = <WhereDateFieldsAreAdded />;
  }

  return (
    <>
      <Select
        {...control}
        options={options}
        value={value}
        onChange={onChange}
        disabled={disabled}
        placeholder={t("auto.dateField.placeholder")}
      />
      {hint && <p className="t-caption param-hint">{hint}</p>}
    </>
  );
}
