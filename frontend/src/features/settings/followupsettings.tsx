import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { NumberSettingRow } from "../../design-system/numbersetting";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList } from "../../design-system/settingrow";
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
