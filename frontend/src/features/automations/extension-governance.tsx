// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { describedByIds } from "../../design-system/describedby";
import { TextInput } from "../../design-system/field";
import { PanelBody } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import {
  useExtensionSettings,
  useUpdateExtensionSettings,
} from "../../lib/extensions.queries";

// What the core does with one unit's feed. Today that is one decision: when a
// feed offers a value for a field a user last edited, it leaves the field
// (off, the default) or overwrites it (on). The server lets only an admin
// change it, so the switch is offered to admins and read-only to the rest.
// Below it come the settings the unit declares as data; the server sends those
// to an admin only, so a reader who is not one sees none.
export function UnitGovernance({
  unit,
  canManage,
}: Readonly<{ unit: string; canManage: boolean }>) {
  const t = useT();
  const query = useExtensionSettings(unit, true);
  const update = useUpdateExtensionSettings(unit);
  const refusalId = useId();
  const save = (key: string, value: string) =>
    update.mutate({
      overwrites_edited_fields: query.data?.overwrites_edited_fields ?? false,
      declared: [{ key, value }],
    });
  return (
    <>
      <PanelBody>
        <SettingList>
          <SettingRow
            label={t("extGovernance.overwrite")}
            description={t("extGovernance.overwriteHint")}
            control={(control) => (
              <Switch
                describedBy={describedByIds(
                  control["aria-describedby"],
                  update.isError && refusalId,
                )}
                label={t("extGovernance.overwrite")}
                labelHidden
                checked={query.data?.overwrites_edited_fields}
                pending={update.isPending || query.isPending}
                reason={canManage ? undefined : t("extGovernance.readOnly")}
                testId={`ext-overwrite-${unit}`}
                onChange={(next) =>
                  update.mutate({ overwrites_edited_fields: next })
                }
              />
            )}
          />
          {query.data?.declared?.map((setting) => (
            <SettingRow
              key={setting.key}
              label={setting.label}
              description={setting.hint}
              control={(control) =>
                setting.kind === "switch" ? (
                  <Switch
                    label={setting.label ?? setting.key}
                    labelHidden
                    checked={setting.value === "true"}
                    pending={update.isPending}
                    testId={`ext-setting-${unit}-${setting.key}`}
                    onChange={(next) => save(setting.key, String(next))}
                  />
                ) : (
                  <TextInput
                    id={control.id}
                    aria-labelledby={control["aria-labelledby"]}
                    aria-describedby={control["aria-describedby"]}
                    data-testid={`ext-setting-${unit}-${setting.key}`}
                    defaultValue={setting.value}
                    key={setting.value}
                    maxLength={200}
                    onBlur={(event) => {
                      if (event.currentTarget.value !== setting.value) {
                        save(setting.key, event.currentTarget.value);
                      }
                    }}
                  />
                )
              }
            />
          ))}
        </SettingList>
      </PanelBody>
      <PanelNotices
        refused={
          update.isError
            ? {
                title: t("extGovernance.notSaved"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </>
  );
}
