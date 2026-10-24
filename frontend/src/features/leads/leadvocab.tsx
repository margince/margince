// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { describedByIds } from "../../design-system/describedby";
import { NumberSettingRow } from "../../design-system/numbersetting";
import { Panel } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useToast } from "../../design-system/toast";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useLeadSettings, useUpdateLeadSettings } from "./leadhandling";
import { useVocabGrants } from "./leadvocab.rows";

export { LeadOriginsCard } from "./leadvocab.origins";
// Every role reads these cards and only the custom_field write verbs change
// them, so a control disables rather than hide.
export { LeadDisqualifyReasonsCard } from "./leadvocab.reasons";

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
  const grants = useVocabGrants("leads");
  const { canEdit } = grants;
  const refusal = grants.refusal && t("leadHandling.readOnly");
  const query = useLeadSettings();
  const update = useUpdateLeadSettings();
  const toast = useToast();
  const refusalId = useId();
  const refusedFor = (
    setting: "first_response_enabled" | "first_response_target_minutes",
  ) =>
    update.isError && update.variables?.[setting] !== undefined
      ? refusalId
      : undefined;
  const readOnly = refusal || undefined;
  return (
    <Panel title={t("leadHandling.title")}>
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
                  describedBy={describedByIds(
                    control["aria-describedby"],
                    refusedFor("first_response_enabled"),
                  )}
                  label={t("leadHandling.firstResponse")}
                  labelHidden
                  checked={settings.first_response_enabled}
                  pending={update.isPending}
                  disabled={!canEdit}
                  reason={lockReasonOr(
                    settings.locks,
                    "contacts.first_response_enabled",
                    t,
                    readOnly,
                  )}
                  testId="lead-first-response-switch"
                  onChange={(next) =>
                    update.mutate(
                      { first_response_enabled: next },
                      { onSuccess: () => toast.show(t("settings.saved")) },
                    )
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
              reason={lockReasonOr(
                settings.locks,
                "contacts.first_response_target_minutes",
                t,
                readOnly,
              )}
              describedBy={refusedFor("first_response_target_minutes")}
              onCommit={(minutes) =>
                update.mutateAsync({ first_response_target_minutes: minutes })
              }
            />
          </SettingList>
        )}
      </QueryGate>
      {/* Each row carries its own refusal, so the card names no posture. */}
      <PanelNotices
        refused={
          update.isError
            ? {
                title: t("leadSources.notSaved"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </Panel>
  );
}
