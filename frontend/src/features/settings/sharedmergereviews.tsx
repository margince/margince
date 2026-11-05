// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../../api/client";
import { describedByIds } from "../../design-system/describedby";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCanMutate, useHoldsAdminRole } from "../../lib/capability";
import { unwrap } from "../../lib/problem";

const SETTINGS_KEY = ["dedupe-settings"] as const;

function useSharedMergeReviewSettings() {
  return useQuery({
    queryKey: SETTINGS_KEY,
    queryFn: async () => unwrap(await api.GET("/dedupe/settings")),
  });
}

function useUpdateSharedMergeReviews() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (enabled: boolean) =>
      unwrap(await api.PUT("/dedupe/settings", { body: { enabled } })),
    onSuccess: (saved) => {
      client.setQueryData(SETTINGS_KEY, saved);
      // Every open case states whether this switch lets its owners merge it.
      void client.invalidateQueries({ queryKey: ["contact-case"] });
    },
  });
}

// Every seat reads the switch, because a case review says who may settle it by
// it. The server lets only the admin role flip it.
export function SharedMergeReviewsCard() {
  const t = useT();
  const admin = useHoldsAdminRole();
  const mutable = useCanMutate();
  const canEdit = admin && mutable;
  const query = useSharedMergeReviewSettings();
  const update = useUpdateSharedMergeReviews();
  const refusalId = useId();
  return (
    <Panel title={t("sharedMergeReviews.title")}>
      <PanelBody>
        <PanelIntro>{t("sharedMergeReviews.hint")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("sharedMergeReviews.title")}>
        {(settings) => (
          <SettingList bleed="settings">
            <SettingRow
              label={t("sharedMergeReviews.switch")}
              control={(control) => (
                <Switch
                  describedBy={describedByIds(
                    control["aria-describedby"],
                    update.isError && refusalId,
                  )}
                  label={t("sharedMergeReviews.switch")}
                  labelHidden
                  checked={settings.enabled}
                  pending={update.isPending}
                  reason={lockReasonOr(
                    settings.locks,
                    "contacts.shared_merge_reviews",
                    t,
                    canEdit ? undefined : t("sharedMergeReviews.readOnly"),
                  )}
                  testId="shared-merge-reviews-switch"
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
            ? {
                title: t("sharedMergeReviews.notSaved"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </Panel>
  );
}
