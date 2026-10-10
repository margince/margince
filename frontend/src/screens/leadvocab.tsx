// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCanWrite } from "../app/capability";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { QueryGate } from "./common";
import { useLeadSettings, useUpdateLeadSettings } from "./leadsources";
import { VocabNotices } from "./leadvocab.rows";

// Every role reads these cards and only the custom_field write verbs change
// them, so a control disables with the reason rather than hide.
export { LeadDisqualifyReasonsCard } from "./leadvocab.reasons";
export { LeadSourcesCard } from "./leadvocab.sources";

// The first-response target's bounds (15 minutes to 7 days), checked here so a
// refusal never leaves the page. backend/gates/settingbounds_test.go holds them
// to the ones the server enforces.
const TARGET_BOUNDS = {
  first_response_target_minutes: { min: 15, max: 10_080 },
} as const;

// The first-response target: off by default, and the number is the
// installation's own. The switch writes when flipped; the target commits on
// Enter or blur like a rename, only once the value is a whole number in range.
//
// Two settings, two rows — this card is the simple posture on the tab, and it
// holds no list, no form and no dialog. A switch and a bounded number each
// ANSWER their row's question, so both sit in the right column and the reader
// audits the pair down one column.
export function LeadHandlingCard() {
  const t = useT();
  const canEdit = useCanWrite("custom_field", "update");
  const query = useLeadSettings();
  const update = useUpdateLeadSettings();
  return (
    <Panel title={t("leadHandling.title")}>
      <PanelBody>
        <PanelIntro>{t("leadHandling.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("leadHandling.title")}>
        {(settings) => (
          <SettingList bleed="settings">
            {/* The posture comes first: the number below is only a
                judgement the switch above it makes readable. */}
            <SettingRow
              label={t("leadHandling.firstResponse")}
              description={t("leadHandling.firstResponseHint")}
              control={(control) => (
                // Without the row's description, a screen reader never
                // hears what this switch does.
                <Switch
                  describedBy={control["aria-describedby"]}
                  label={t("leadHandling.firstResponse")}
                  labelHidden
                  checked={settings.first_response_enabled}
                  pending={update.isPending}
                  reason={canEdit ? undefined : t("leadSources.readOnly")}
                  testId="lead-first-response-switch"
                  onChange={(next) =>
                    update.mutate({ first_response_enabled: next })
                  }
                />
              )}
            />
            <NumberSettingRow
              label={t("leadHandling.targetMinutes")}
              description={t("leadHandling.targetHint")}
              testId="lead-first-response-target"
              value={settings.first_response_target_minutes}
              {...TARGET_BOUNDS.first_response_target_minutes}
              refusal={t("leadHandling.targetOutOfRange")}
              disabled={
                !canEdit || !settings.first_response_enabled || update.isPending
              }
              onCommit={(minutes) =>
                update.mutate({ first_response_target_minutes: minutes })
              }
            />
          </SettingList>
        )}
      </QueryGate>
      {/* This card's rows are switches every seat may flip, so no posture. */}
      <VocabNotices error={update.isError ? update.error : undefined} />
    </Panel>
  );
}
