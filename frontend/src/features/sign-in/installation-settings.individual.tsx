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

type SwitchKey = "individual_performance" | "forecast_self_correcting";

type SwitchCopy = Readonly<{
  title: string;
  name: string;
  hint: string;
  on: string;
  off: string;
}>;

type IndividualPerformanceUpdate = Readonly<{
  mutate: (patch: Patch) => void;
  isPending: boolean;
  isError: boolean;
  error: unknown;
}>;

type SwitchRowsProps = Readonly<{
  settings: InstallationSettings;
  canManage: boolean;
  /** The panel's sentence that refuses every change to a reader who may not. */
  denialId: string;
  update: IndividualPerformanceUpdate;
}>;

/** The works-council switch: whether anyone sees a colleague's figures. */
export function IndividualPerformanceRows(props: SwitchRowsProps) {
  const t = useT();
  return (
    <InstallationSwitchRows
      {...props}
      settingKey="individual_performance"
      copy={{
        title: t("installationSettings.individualTitle"),
        name: t("installationSettings.individualPerformance"),
        hint: t("installationSettings.individualHint"),
        on: t("installationSettings.individualOn"),
        off: t("installationSettings.individualOff"),
      }}
    />
  );
}

/** Whether the forecast moves each stage's chance with recorded outcomes. */
export function ForecastSelfCorrectingRows(props: SwitchRowsProps) {
  const t = useT();
  return (
    <InstallationSwitchRows
      {...props}
      settingKey="forecast_self_correcting"
      copy={{
        title: t("installationSettings.selfCorrectingTitle"),
        name: t("installationSettings.selfCorrecting"),
        hint: t("installationSettings.selfCorrectingHint"),
        on: t("installationSettings.selfCorrectingOn"),
        off: t("installationSettings.selfCorrectingOff"),
      }}
    />
  );
}

/** One installation-wide switch that writes when turned. */
function InstallationSwitchRows({
  settings,
  canManage,
  denialId,
  update,
  settingKey,
  copy,
}: SwitchRowsProps & Readonly<{ settingKey: SwitchKey; copy: SwitchCopy }>) {
  const t = useT();
  const refusalId = useId();
  const name = copy.name;
  const checked = settings[settingKey];
  return (
    <>
      <PanelGroupHead title={copy.title} level="h3" />
      <PanelBody>
        <PanelIntro>{copy.hint}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={name}
          description={checked ? copy.on : copy.off}
          control={(control) => (
            <Switch
              describedBy={describedByIds(
                control["aria-describedby"],
                !canManage && denialId,
                update.isError && refusalId,
              )}
              label={name}
              labelHidden
              checked={checked}
              disabled={!canManage}
              pending={update.isPending}
              reason={lockReasonOr(
                settings.locks,
                `installation.${settingKey}`,
                t,
                undefined,
              )}
              onChange={(next) => update.mutate({ [settingKey]: next })}
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
