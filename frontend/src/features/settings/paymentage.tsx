// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ErrorLine } from "../../design-system/errorline";
import { NumberSettingRow } from "../../design-system/numbersetting";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList } from "../../design-system/settingrow";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { useUpdateInstallationSettings } from "../sign-in/installation-settings";

const PAYMENT_AGE_BOUNDS = {
  payment_long_overdue_days: { min: 1, max: 3650 },
} as const;

export function PaymentAgeSettings() {
  const t = useT();
  const canRead = useCan("installation_settings", "read");
  const canWrite = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings(canRead);
  const update = useUpdateInstallationSettings(() => undefined);
  if (!canRead) return null;
  return (
    <Panel title={t("payments.ageTitle")}>
      <PanelBody>
        <QueryGate query={query} pendingLabel={t("payments.ageTitle")}>
          {(settings) => (
            <SettingList>
              <NumberSettingRow
                label={t("payments.ageLabel")}
                description={t("payments.ageHelp")}
                value={settings.payment_long_overdue_days ?? 365}
                {...PAYMENT_AGE_BOUNDS.payment_long_overdue_days}
                refusal={t("payments.ageRefusal")}
                disabled={!canWrite || update.isPending}
                reason={lockReasonOr(
                  settings.locks,
                  "installation.payment_long_overdue_days",
                  t,
                  undefined,
                )}
                onCommit={(days) =>
                  update.mutateAsync({ payment_long_overdue_days: days })
                }
              />
            </SettingList>
          )}
        </QueryGate>
        <ErrorLine error={update.error} />
      </PanelBody>
    </Panel>
  );
}
