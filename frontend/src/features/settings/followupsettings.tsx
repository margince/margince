import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { describedByIds } from "../../design-system/describedby";
import { ErrorLine } from "../../design-system/errorline";
import { NumberSettingRow } from "../../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { worklistKey } from "../worklist/worklist.queries";

type FollowUpSettings = components["schemas"]["FollowUpSettings"];

const FOLLOW_UP_SETTINGS_KEY = ["follow-up-settings"] as const;

// The bounds the server enforces, mirrored so an out-of-range number is
// refused here; backend/gates/settingbounds_test.go holds the two together.
const FOLLOW_UP_BOUNDS = {
  follow_up_after_days: { min: 1, max: 30 },
} as const;

function useFollowUpSettings() {
  return useQuery({
    queryKey: FOLLOW_UP_SETTINGS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/activities/follow-up-settings",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useUpdateFollowUpSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: FollowUpSettings) => {
      return unwrap(
        await api.PATCH("/activities/follow-up-settings", { body }),
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: FOLLOW_UP_SETTINGS_KEY });
      // The worklist's follow-up rows are cut at this window.
      void queryClient.invalidateQueries({ queryKey: worklistKey });
    },
  });
}

// How long a message a seat sent to a customer may stay unanswered before
// their Home worklist reminds them to follow up.
export function FollowUpSettingsCard() {
  const t = useT();
  const canEdit = useCanWrite("installation_settings", "update");
  // The read-only line waits for /me, so a reader who may edit never sees it.
  const readOnly = useMe().isSuccess && !canEdit;
  const query = useFollowUpSettings();
  const update = useUpdateFollowUpSettings();
  return (
    <Panel title={t("followUpSettings.title")}>
      <PanelBody>
        {readOnly && <p>{t("installationSettings.readOnly")}</p>}
        <QueryGate query={query} pendingLabel={t("followUpSettings.title")}>
          {(settings) => (
            <SettingList>
              <NumberSettingRow
                label={t("followUpSettings.days")}
                description={t("followUpSettings.daysHint")}
                testId="follow-up-after-days"
                value={settings.follow_up_after_days}
                {...FOLLOW_UP_BOUNDS.follow_up_after_days}
                refusal={t("followUpSettings.outOfRange")}
                disabled={!canEdit || update.isPending}
                reason={lockReasonOr(
                  settings.locks,
                  "activities.follow_up_after_days",
                  t,
                  undefined,
                )}
                onCommit={(days) =>
                  update.mutateAsync({ follow_up_after_days: days })
                }
              />
            </SettingList>
          )}
        </QueryGate>
        <ErrorLine error={update.isError ? update.error : undefined} />
      </PanelBody>
    </Panel>
  );
}

// Whether the AI reads private mail for requests, and whether a quotation
// request found there is shared with the workspace. Both ride the follow-up
// settings because they are the activities module's; the PATCH carries the
// window unchanged beside them.
export function PrivateMailRequestsCard() {
  const t = useT();
  const canEdit = useCanWrite("installation_settings", "update");
  const query = useFollowUpSettings();
  const update = useUpdateFollowUpSettings();
  const refusalId = useId();
  return (
    <Panel title={t("privateMailRequests.title")}>
      <PanelBody>
        <PanelIntro>{t("privateMailRequests.hint")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("privateMailRequests.title")}>
        {(settings) => (
          <SettingList bleed="settings">
            <SettingRow
              label={t("privateMailRequests.switch")}
              control={(control) => (
                <Switch
                  describedBy={describedByIds(
                    control["aria-describedby"],
                    update.isError && refusalId,
                  )}
                  label={t("privateMailRequests.switch")}
                  labelHidden
                  checked={settings.private_mail_requests ?? true}
                  pending={update.isPending}
                  reason={lockReasonOr(
                    settings.locks,
                    "activities.private_mail_requests",
                    t,
                    canEdit ? undefined : t("privateMailRequests.readOnly"),
                  )}
                  testId="private-mail-requests-switch"
                  onChange={(next) =>
                    update.mutate({
                      follow_up_after_days: settings.follow_up_after_days,
                      private_mail_requests: next,
                    })
                  }
                />
              )}
            />
            <SettingRow
              label={t("privateMailRequests.share")}
              description={t("privateMailRequests.shareHint")}
              control={(control) => (
                <Switch
                  describedBy={describedByIds(
                    control["aria-describedby"],
                    update.isError && refusalId,
                  )}
                  label={t("privateMailRequests.share")}
                  labelHidden
                  checked={settings.share_private_quote_requests ?? true}
                  pending={update.isPending}
                  reason={lockReasonOr(
                    settings.locks,
                    "activities.share_private_quote_requests",
                    t,
                    canEdit ? undefined : t("privateMailRequests.readOnly"),
                  )}
                  testId="share-private-quote-requests-switch"
                  onChange={(next) =>
                    update.mutate({
                      follow_up_after_days: settings.follow_up_after_days,
                      share_private_quote_requests: next,
                    })
                  }
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
                title: t("privateMailRequests.notSaved"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </Panel>
  );
}
