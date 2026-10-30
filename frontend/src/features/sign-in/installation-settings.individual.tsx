// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import type { components } from "../../api/schema";
import { describedByIds } from "../../design-system/describedby";
import {
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

const KEY = "individual_performance";

type IndividualPerformanceUpdate = Readonly<{
  mutate: (patch: Patch) => void;
  isPending: boolean;
  isError: boolean;
  error: unknown;
}>;

/** The works-council switch: whether anyone sees a colleague's figures. */
export function IndividualPerformanceRows({
  settings,
  canManage,
  denialId,
  update,
}: Readonly<{
  settings: InstallationSettings;
  canManage: boolean;
  /** The panel's sentence that refuses every change to a reader who may not. */
  denialId: string;
  update: IndividualPerformanceUpdate;
}>) {
  const t = useT();
  const refusalId = useId();
  const name = t("installationSettings.individualPerformance");
  return (
    <>
      <PanelGroupHead
        title={t("installationSettings.individualTitle")}
        level="h3"
      />
      <PanelBody>
        <PanelIntro>{t("installationSettings.individualHint")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={name}
          description={t(
            settings.individual_performance
              ? "installationSettings.individualOn"
              : "installationSettings.individualOff",
          )}
          control={(control) => (
            <Switch
              describedBy={describedByIds(
                control["aria-describedby"],
                !canManage && denialId,
                update.isError && refusalId,
              )}
              label={name}
              labelHidden
              checked={settings.individual_performance}
              disabled={!canManage}
              pending={update.isPending}
              reason={lockReasonOr(
                settings.locks,
                `installation.${KEY}`,
                t,
                undefined,
              )}
              onChange={(next) => update.mutate({ [KEY]: next })}
            />
          )}
        />
      </SettingList>
      <PanelNotices
        refused={
          update.isError
            ? {
                title: t("installationSettings.saveFailed"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </>
  );
}
