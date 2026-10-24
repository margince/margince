// What the product may send this reader about their day and their week.
//
// The brief and the weekly show on Home either way. These rows switch off only
// the email that points to them, so they sit with the reader's own notifications.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { PendingBody } from "../../design-system/atoms";
import { describedByIds } from "../../design-system/describedby";
import { Panel } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { useSaveDraft } from "../../design-system/savepage";
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
 * it, and saving a pick is what turns a default into a decision.
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
  // Only the picked keys: the endpoint is a patch, and a key left at its
  // default must stay "never chose" rather than become a decision.
  const [draft, setDraft] = useState<BriefDelivery>({});
  const save = useMutation({
    mutationFn: async (patch: BriefDelivery) => {
      unwrap(await api.PUT("/me/brief-delivery", { body: patch }));
    },
    onSuccess: async () => {
      // Awaited before the draft drops, so the rows never flash the old choice.
      await queryClient.invalidateQueries({
        queryKey: ["me", "brief-delivery"],
      });
      setDraft({});
    },
  });

  const stored = settings.data ?? {};
  const shown: BriefDelivery = { ...stored, ...draft };
  const changed = Object.fromEntries(
    Object.entries(draft).filter(
      ([key, value]) => value !== stored[key as keyof BriefDelivery],
    ),
  );
  useSaveDraft({
    dirty: Object.keys(changed).length > 0,
    save: () => save.mutateAsync(changed),
    discard: () => {
      save.reset();
      setDraft({});
    },
  });

  const chosen = (value: Choice | undefined): Choice => value ?? "email";
  // The refused write's keys name themselves. The bar holds the one summary, and
  // each control that was in the write points at the notice under the list.
  const refusedBy = (setting: keyof BriefDelivery) =>
    save.isError && save.variables?.[setting] !== undefined
      ? refusalId
      : undefined;
  const options = CHOICES.map((choice) => ({
    value: choice,
    label: t(choice === "email" ? "delivery.byEmail" : "delivery.none"),
  }));
  // Narrowed through the same list the options are built from, so nothing is
  // drafted that the control was never offering.
  const pick =
    (key: "morning_brief_delivery" | "weekly_delivery") => (next: string) => {
      const picked = CHOICES.find((option) => option === next);
      if (picked) setDraft((held) => ({ ...held, [key]: picked }));
    };

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
              aria-describedby={describedByIds(
                control["aria-describedby"],
                refusedBy("morning_brief_delivery"),
              )}
              className="settingrow-measure"
              value={chosen(shown.morning_brief_delivery)}
              onChange={pick("morning_brief_delivery")}
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
              aria-describedby={describedByIds(
                control["aria-describedby"],
                refusedBy("weekly_delivery"),
              )}
              className="settingrow-measure"
              value={chosen(shown.weekly_delivery)}
              onChange={pick("weekly_delivery")}
              options={options}
            />
          )}
        />
        <RoutineDeliveryRows
          settings={shown}
          refusalFor={refusedBy}
          save={(patch) => setDraft((held) => ({ ...held, ...patch }))}
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
