// What the product may send this reader about their day and their week.
//
// The brief and the weekly show on Home either way. These rows switch off only
// the email that points to them, so they sit with the reader's own notifications.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { PendingBody } from "../../design-system/atoms";
import { describedByIds } from "../../design-system/describedby";
import { Panel } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { RoutineDeliveryRows } from "./routinedelivery";

type BriefDelivery = components["schemas"]["BriefDelivery"];

/** The two delivery choices, in the order the dropdown offers them. */
const CHOICES = ["email", "none"] as const;
type Choice = (typeof CHOICES)[number];

export function BriefDeliveryPanel() {
  const t = useT();
  return (
    <Panel title={t("delivery.panelTitle")}>
      <BriefDeliveryRows />
    </Panel>
  );
}

/**
 * The delivery settings, as rows in the summaries panel.
 *
 * A member who has never chosen sees the dropdown on its DEFAULT rather than on
 * a blank: the server distinguishes "never chose" from "chose none", and the
 * screen's job is to show what happens today, which for an unchosen setting is
 * whatever the installation does. Sending is that default, so the control shows
 * it — and touching the control is what turns a default into a decision.
 */
function BriefDeliveryRows() {
  const t = useT();
  const queryClient = useQueryClient();
  const refusalId = useId();
  const settings = useQuery({
    queryKey: ["me", "brief-delivery"],
    queryFn: async () => {
      const data = unwrap(await api.GET("/me/brief-delivery"));
      return data ?? {};
    },
  });
  // Each control sends only ITSELF. The endpoint is a patch, so a row that sent
  // the whole form would let a stale render of one dropdown overwrite a choice
  // the reader had just made in another.
  const save = useMutation({
    mutationFn: async (patch: BriefDelivery) => {
      unwrap(await api.PUT("/me/brief-delivery", { body: patch }));
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["me", "brief-delivery"],
      });
    },
  });

  const chosen = (value: Choice | undefined): Choice => value ?? "email";
  const describedBy = (own: string | undefined, setting: keyof BriefDelivery) =>
    describedByIds(
      own,
      save.isError && save.variables?.[setting] !== undefined && refusalId,
    );
  const options = CHOICES.map((choice) => ({
    value: choice,
    label: t(choice === "email" ? "delivery.byEmail" : "delivery.none"),
  }));

  if (settings.isPending)
    return <PendingBody label={t("delivery.panelTitle")} />;
  if (settings.isError)
    return (
      <PanelNotices
        refused={{ title: t("delivery.loadFailed"), error: settings.error }}
      />
    );

  return (
    <>
      <SettingList bleed="settings">
        <SettingRow
          label={t("delivery.morningLabel")}
          description={t("delivery.morningHelp")}
          testId="delivery-morning"
          control={(control) => (
            <Select
              {...control}
              disabled={save.isPending}
              aria-describedby={describedBy(
                control["aria-describedby"],
                "morning_brief_delivery",
              )}
              className="settingrow-measure"
              value={chosen(settings.data?.morning_brief_delivery)}
              onChange={(next) => {
                // Narrowed through the same list the options are built from, so
                // nothing is acted on that the control was never offering.
                const picked = CHOICES.find((option) => option === next);
                if (picked) {
                  save.mutate({ morning_brief_delivery: picked });
                }
              }}
              options={options}
            />
          )}
        />
        <SettingRow
          label={t("delivery.weeklyLabel")}
          description={t("delivery.weeklyHelp")}
          testId="delivery-weekly"
          control={(control) => (
            <Select
              {...control}
              disabled={save.isPending}
              aria-describedby={describedBy(
                control["aria-describedby"],
                "weekly_delivery",
              )}
              className="settingrow-measure"
              value={chosen(settings.data?.weekly_delivery)}
              onChange={(next) => {
                const picked = CHOICES.find((option) => option === next);
                if (picked) {
                  save.mutate({ weekly_delivery: picked });
                }
              }}
              options={options}
            />
          )}
        />
        <RoutineDeliveryRows
          settings={settings.data}
          disabled={save.isPending}
          refusalFor={(field) =>
            save.isError && save.variables?.[field] !== undefined
              ? refusalId
              : undefined
          }
          save={(patch) => save.mutate(patch)}
        />
      </SettingList>
      <PanelNotices
        refused={
          save.isError
            ? {
                title: t("settings.saveFailed"),
                error: save.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </>
  );
}
