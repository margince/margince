// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";

/**
 * What stops working without this answer, said where the answer is being given.
 * `warning` and `standing`, not danger: a "no" is the rep's authority to give,
 * and the notice reports its cost rather than a fault.
 */
export function GrantCostNotice() {
  const t = useT();
  return (
    <Callout
      kind="standing"
      tone="warning"
      title={t("overnightGrant.dangerTitle")}
    >
      {t("overnightGrant.danger")}
    </Callout>
  );
}

/**
 * The onboarding question: a preselected Switch row for the caller's SettingList.
 *
 * Preselected: the features it feeds are the ones the product opens on. An
 * off default would ship a morning brief that stays empty for reasons nobody
 * is told. Turning it off is warned about, never blocked.
 *
 * It does not write. The answer rides with the step's continue (`checked` /
 * `onChange`), so a rep who abandons onboarding here has granted nothing.
 */
export function OvernightGrantChoice({
  checked,
  onChange,
  failed = false,
}: Readonly<{
  checked: boolean;
  onChange: (next: boolean) => void;
  /** The answer could not be recorded when the step completed. Shown rather
   * than swallowed, so the rep does not leave believing they had answered. */
  failed?: boolean;
}>) {
  const t = useT();
  return (
    <>
      <SettingRow
        label={t("overnightGrant.label")}
        description={t("overnightGrant.choiceHelp")}
        control={(control) => (
          <Switch
            describedBy={control["aria-describedby"]}
            testId="overnight-grant-choice"
            label={t("overnightGrant.label")}
            labelHidden
            checked={checked}
            onChange={onChange}
          />
        )}
      />
      {!checked && <GrantCostNotice />}
      {failed && (
        <Callout
          kind="outcome"
          tone="danger"
          title={t("overnightGrant.saveFailedTitle")}
        >
          {t("overnightGrant.saveFailed")}
        </Callout>
      )}
    </>
  );
}
