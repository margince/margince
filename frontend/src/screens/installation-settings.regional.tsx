// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";

type InstallationSettings = components["schemas"]["InstallationSettings"];

export function RegionalSettingsRows({
  settings,
  editVerb,
}: Readonly<{
  settings: InstallationSettings;
  editVerb: (fact: "date_format" | "time_format", label: string) => ReactNode;
}>) {
  const t = useT();
  return (
    <>
      <SettingRow
        label={t("installationSettings.dateFormat")}
        value={t(
          `installationSettings.dateFormat.${settings.date_format ?? "locale"}`,
        )}
        control={editVerb("date_format", t("installationSettings.dateFormat"))}
      />
      <SettingRow
        label={t("installationSettings.timeFormat")}
        value={t(
          `installationSettings.timeFormat.${settings.time_format ?? "locale"}`,
        )}
        control={editVerb("time_format", t("installationSettings.timeFormat"))}
      />
    </>
  );
}

export function RegionalSettingsFields({
  draft,
  canManage,
  refused,
  onChange,
}: Readonly<{
  draft: InstallationSettings;
  canManage: boolean;
  refused: Map<string, string>;
  onChange: (next: InstallationSettings) => void;
}>) {
  const t = useT();
  return (
    <>
      <div data-fact="date_format">
        <Field
          label={t("installationSettings.dateFormat")}
          hint={t("installationSettings.formatsHint")}
          error={refused.get("date_format")}
        >
          {(control) => (
            <Select
              {...control}
              value={draft.date_format ?? "locale"}
              disabled={!canManage}
              options={[
                {
                  value: "locale",
                  label: t("installationSettings.dateFormat.locale"),
                },
                {
                  value: "dmy",
                  label: t("installationSettings.dateFormat.dmy"),
                },
                {
                  value: "mdy",
                  label: t("installationSettings.dateFormat.mdy"),
                },
                {
                  value: "ymd",
                  label: t("installationSettings.dateFormat.ymd"),
                },
              ]}
              onChange={(picked) => {
                if (
                  picked === "locale" ||
                  picked === "dmy" ||
                  picked === "mdy" ||
                  picked === "ymd"
                )
                  onChange({ ...draft, date_format: picked });
              }}
            />
          )}
        </Field>
      </div>
      <div data-fact="time_format">
        <Field
          label={t("installationSettings.timeFormat")}
          hint={t("installationSettings.formatsHint")}
          error={refused.get("time_format")}
        >
          {(control) => (
            <Select
              {...control}
              value={draft.time_format ?? "locale"}
              disabled={!canManage}
              options={[
                {
                  value: "locale",
                  label: t("installationSettings.timeFormat.locale"),
                },
                {
                  value: "24h",
                  label: t("installationSettings.timeFormat.24h"),
                },
                {
                  value: "12h",
                  label: t("installationSettings.timeFormat.12h"),
                },
              ]}
              onChange={(picked) => {
                if (picked === "locale" || picked === "24h" || picked === "12h")
                  onChange({ ...draft, time_format: picked });
              }}
            />
          )}
        </Field>
      </div>
    </>
  );
}
