// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { describedByIds } from "../../design-system/describedby";
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
export function UnitGovernance({
  unit,
  canManage,
}: Readonly<{ unit: string; canManage: boolean }>) {
  const t = useT();
  const query = useExtensionSettings(unit, true);
  const update = useUpdateExtensionSettings(unit);
  const refusalId = useId();
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
