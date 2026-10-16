// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import { useCanMutate, useHoldsAdminRole } from "../../lib/capability";
import {
  useLegalReviewSettings,
  useUpdateLegalReviewSettings,
} from "./legalreviews.queries";

// Every seat reads the switch, since the deal page follows it. The server lets
// only the admin role flip it; ops also holds installation_settings.
export function LegalReviewSettingsCard() {
  const t = useT();
  const admin = useHoldsAdminRole();
  const mutable = useCanMutate();
  const canEdit = admin && mutable;
  const query = useLegalReviewSettings();
  const update = useUpdateLegalReviewSettings();
  return (
    <Panel title={t("legalReview.settingsTitle")}>
      <PanelBody>
        <PanelIntro>{t("legalReview.settingsHint")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("legalReview.settingsTitle")}>
        {(settings) => (
          <SettingList bleed="settings">
            <SettingRow
              label={t("legalReview.settingsSwitch")}
              control={(control) => (
                <Switch
                  describedBy={control["aria-describedby"]}
                  label={t("legalReview.settingsSwitch")}
                  labelHidden
                  checked={settings.enabled}
                  pending={update.isPending}
                  reason={
                    canEdit ? undefined : t("legalReview.settingsReadOnly")
                  }
                  testId="legal-review-switch"
                  onChange={(next) => update.mutate(next)}
                />
              )}
            />
          </SettingList>
        )}
      </QueryGate>
      <PanelNotices
        refused={
          update.isError
            ? { title: t("legalReview.notSaved"), error: update.error }
            : undefined
        }
      />
    </Panel>
  );
}
