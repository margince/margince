// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { AutonomyDot } from "../design-system/trust";
import { useT } from "../i18n";
import "./settings-agents.css";

// Reference for the tiers the tools are marked with. Advancing a stage is locked
// at its floor, waiting only when the move closes the deal.
export function AutonomyTiersCard() {
  const t = useT();
  return (
    <Panel title={t("settings.autonomy")}>
      <PanelBody>
        <PanelIntro>{t("settings.autonomySub")}</PanelIntro>
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={t("settings.tierRead")}
          control={<AutonomyDot tier="auto" withLabel />}
        />
        {/* Granting the `send` scope is the approval, so a send runs at once. */}
        <SettingRow
          label={t("settings.tierSend")}
          control={<AutonomyDot tier="auto" withLabel />}
        />
        <SettingRow
          label={t("settings.tierWait")}
          control={<AutonomyDot tier="confirm" withLabel />}
        />
        <SettingRow
          label={t("settings.tierAdvance")}
          control={
            <span className="agents-tier">
              <AutonomyDot tier="confirm" withLabel />
              <Badge tone="warning">{t("settings.locked")}</Badge>
            </span>
          }
        />
      </SettingList>
    </Panel>
  );
}
