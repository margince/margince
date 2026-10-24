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
import type { MessageKey } from "../../i18n/en";
import {
  MESSENGER_SETTING,
  MESSENGERS,
  type Messenger,
} from "../../lib/callmessengers";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

const LABEL = {
  whatsapp: "installationSettings.callWhatsApp",
  telegram: "installationSettings.callTelegram",
  zalo: "installationSettings.callZalo",
} as const satisfies Record<Messenger, MessageKey>;

type CallingUpdate = Readonly<{
  mutate: (patch: Patch) => void;
  isPending: boolean;
  isError: boolean;
  variables: Patch | undefined;
  error: unknown;
}>;

/** One switch per messenger link; a flip writes that messenger's key alone. */
export function CallingSettingsRows({
  settings,
  canManage,
  denialId,
  update,
}: Readonly<{
  settings: InstallationSettings;
  canManage: boolean;
  /** The panel's sentence that refuses every change to a reader who may not. */
  denialId: string;
  update: CallingUpdate;
}>) {
  const t = useT();
  const refusalId = useId();
  return (
    <>
      <PanelGroupHead
        title={t("installationSettings.callingTitle")}
        level="h3"
      />
      <PanelBody>
        <PanelIntro>{t("installationSettings.callingHint")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        {MESSENGERS.map((messenger) => {
          const key = MESSENGER_SETTING[messenger];
          const name = t(LABEL[messenger]);
          return (
            <SettingRow
              key={messenger}
              label={name}
              control={(control) => (
                <Switch
                  describedBy={describedByIds(
                    control["aria-describedby"],
                    !canManage && denialId,
                    update.isError && refusalId,
                  )}
                  label={name}
                  labelHidden
                  checked={settings[key]}
                  disabled={!canManage}
                  pending={
                    update.isPending && update.variables?.[key] !== undefined
                  }
                  reason={lockReasonOr(
                    settings.locks,
                    `installation.${key}`,
                    t,
                    undefined,
                  )}
                  onChange={(next) => update.mutate({ [key]: next })}
                />
              )}
            />
          );
        })}
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
