import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { ErrorLine } from "../design-system/errorline";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList } from "../design-system/settingrow";
import { useT } from "../i18n";
import { QueryGate, throwProblem, unwrap } from "./common";
import { worklistKey } from "./worklist.queries";

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
  const query = useFollowUpSettings();
  const update = useUpdateFollowUpSettings();
  return (
    <Panel title={t("followUpSettings.title")}>
      <PanelBody>
        <PanelIntro>{t("followUpSettings.sub")}</PanelIntro>
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
                onCommit={(days) =>
                  update.mutate({ follow_up_after_days: days })
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
