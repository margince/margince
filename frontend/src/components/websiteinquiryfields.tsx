// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import {
  MultiSelect,
  Select,
  type SelectOption,
} from "../design-system/select";
import { TimezoneSelect } from "../design-system/timezoneselect";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "../lib/labelfor";
import type {
  WebsiteInquiryMode,
  WebsiteInquiryPolicy,
} from "../lib/websiteinquiries";
import { useRecordOwnerOptions } from "./assigneepicker";
import { useObjectCustomFields } from "./customfields.form";
import {
  RosterFailedLine,
  RosterPartialNote,
  useRosterPartial,
} from "./entityref";

type WebForm = components["schemas"]["WebForm"];

const MODE_LABELS: Readonly<Record<WebsiteInquiryMode, MessageKey>> = {
  one_colleague: "inquiry.mode.one",
  rotate: "inquiry.mode.rotate",
  coordinator: "inquiry.mode.coordinator",
};

function isMode(value: string): value is WebsiteInquiryMode {
  return Object.hasOwn(MODE_LABELS, value);
}

export function inquiryModeLabel(
  mode: WebsiteInquiryMode,
  t: (key: MessageKey) => string,
): string {
  return t(labelFor(MODE_LABELS, mode));
}

function withSavedOptions(
  options: readonly SelectOption[],
  selected: readonly string[],
  missing: string,
): readonly SelectOption[] {
  return [
    ...options,
    ...selected
      .filter(
        (id) => id !== "" && !options.some((option) => option.value === id),
      )
      .map((id) => ({ value: id, label: missing })),
  ];
}

export function WebsiteInquiryFields({
  value,
  form,
  onChange,
}: Readonly<{
  value: WebsiteInquiryPolicy;
  form: WebForm;
  onChange: (next: WebsiteInquiryPolicy) => void;
}>) {
  const t = useT();
  const { options, roster } = useRecordOwnerOptions();
  const partial = useRosterPartial("user", true);
  const customFields = useObjectCustomFields("lead");
  const requestTypes = customFields.fields.filter(
    (field) => field.type === "picklist",
  );
  const owners = value.routing.owners ?? [];
  const colleagues = withSavedOptions(
    options,
    [...owners, value.coordinator_id],
    t("inquiry.colleagueUnavailable"),
  );
  const setOwners = (next: string[]) =>
    onChange({ ...value, routing: { ...value.routing, owners: next } });
  return (
    <>
      <Field label={t("inquiry.form")}>
        {(control) => (
          <Select
            {...control}
            options={[{ value: form.id, label: form.name }]}
            value={form.id}
            onChange={() => {}}
            disabled
          />
        )}
      </Field>
      <Field label={t("inquiry.mode")}>
        {(control) => (
          <Select
            {...control}
            required
            options={Object.entries(MODE_LABELS).map(([mode, label]) => ({
              value: mode,
              label: t(label),
            }))}
            value={value.mode}
            onChange={(mode) => {
              if (!isMode(mode)) return;
              const routing =
                mode === "coordinator"
                  ? {}
                  : {
                      ...value.routing,
                      owners:
                        mode === "one_colleague" ? owners.slice(0, 1) : owners,
                    };
              onChange({ ...value, mode, routing });
            }}
          />
        )}
      </Field>
      {value.mode !== "coordinator" && (
        <Field
          label={t(
            value.mode === "rotate"
              ? "inquiry.colleagues"
              : "inquiry.colleague",
          )}
          hint={t("inquiry.keepOwner")}
        >
          {(control) =>
            value.mode === "rotate" ? (
              <MultiSelect
                {...control}
                required
                options={colleagues}
                values={owners}
                onChange={setOwners}
                placeholder={t("inquiry.chooseColleagues")}
              />
            ) : (
              <Select
                {...control}
                required
                options={colleagues}
                value={owners[0] ?? ""}
                onChange={(id) => setOwners([id])}
                placeholder={t("inquiry.chooseColleague")}
              />
            )
          }
        </Field>
      )}
      <Field
        label={t("inquiry.coordinator")}
        hint={t("inquiry.coordinatorHint")}
      >
        {(control) => (
          <Select
            {...control}
            required
            options={colleagues}
            value={value.coordinator_id}
            onChange={(coordinator_id) =>
              onChange({ ...value, coordinator_id })
            }
            placeholder={t("inquiry.chooseColleague")}
          />
        )}
      </Field>
      <RosterFailedLine kind="user" roster={roster} />
      <RosterPartialNote partial={partial} />
      <Field
        label={t("inquiry.requestTypeField")}
        hint={t("inquiry.requestTypeHint")}
      >
        {(control) => (
          <Select
            {...control}
            options={withSavedOptions(
              [
                { value: "", label: t("inquiry.notSpecified") },
                ...requestTypes.map((field) => ({
                  value: field.column_name,
                  label: field.label,
                })),
              ],
              [value.request_type_field ?? ""],
              t("inquiry.fieldUnavailable"),
            )}
            value={value.request_type_field ?? ""}
            disabled={customFields.loading || customFields.failed}
            onChange={(field) =>
              onChange({ ...value, request_type_field: field || undefined })
            }
          />
        )}
      </Field>
      {customFields.failed && (
        <p role="status">
          {t("inquiry.fieldsFailed")}{" "}
          <Button onClick={customFields.retry}>{t("common.retry")}</Button>
        </p>
      )}
      <Field label={t("inquiry.taskTitle")}>
        {(control) => (
          <TextInput
            {...control}
            required
            maxLength={200}
            value={value.task_title}
            onChange={(event) =>
              onChange({ ...value, task_title: event.target.value })
            }
          />
        )}
      </Field>
      <Field label={t("inquiry.dueDays")} hint={t("inquiry.dueHint")}>
        {(control) => (
          <TextInput
            {...control}
            required
            type="number"
            min={1}
            max={30}
            value={String(value.due_in_days)}
            onChange={(event) =>
              onChange({ ...value, due_in_days: Number(event.target.value) })
            }
          />
        )}
      </Field>
      <Field label={t("auto.deadlineTimezone")}>
        {(control) => (
          <TimezoneSelect
            {...control}
            value={value.timezone}
            onChange={(timezone) => onChange({ ...value, timezone })}
          />
        )}
      </Field>
    </>
  );
}
