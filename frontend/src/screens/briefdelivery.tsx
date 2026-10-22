// What the product may send this reader about their day and their week.
//
// The brief and the weekly are on their screen either way; these rows are about
// the NUDGE toward them, which is the one part a contact is entitled to switch
// off. So they sit under the account's own settings beside the display
// language, not under anything an admin configures for the team.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useSaveDraft } from "../design-system/savepage";
import { Select } from "../design-system/select";
import { SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { unwrap } from "../lib/problem";

type BriefDelivery = components["schemas"]["BriefDelivery"];

/** The two delivery choices, in the order the dropdown offers them. */
const CHOICES = ["email", "none"] as const;
type Choice = (typeof CHOICES)[number];

/**
 * The delivery settings, as rows in the account panel.
 *
 * A member who has never chosen sees the dropdown on its DEFAULT rather than on
 * a blank: the server distinguishes "never chose" from "chose none", and the
 * screen's job is to show what happens today, which for an unchosen setting is
 * whatever the installation does. Sending is that default, so the control shows
 * it, and saving a pick is what turns a default into a decision.
 */
export function BriefDeliveryRows() {
  const t = useT();
  const queryClient = useQueryClient();
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
  const morning = draft.morning_brief_delivery ?? stored.morning_brief_delivery;
  const weekly = draft.weekly_delivery ?? stored.weekly_delivery;
  const changed: BriefDelivery = {
    ...(morning !== stored.morning_brief_delivery && {
      morning_brief_delivery: morning,
    }),
    ...(weekly !== stored.weekly_delivery && { weekly_delivery: weekly }),
  };
  useSaveDraft({
    dirty: Object.keys(changed).length > 0,
    save: () => save.mutateAsync(changed),
    discard: () => {
      save.reset();
      setDraft({});
    },
  });

  const chosen = (value: Choice | undefined): Choice => value ?? "email";
  const options = CHOICES.map((choice) => ({
    value: choice,
    label: t(choice === "email" ? "delivery.byEmail" : "delivery.none"),
  }));
  // Narrowed through the same list the options are built from, so nothing is
  // drafted that the control was never offering.
  const pick = (key: keyof BriefDelivery) => (next: string) => {
    const picked = CHOICES.find((option) => option === next);
    if (picked) setDraft((held) => ({ ...held, [key]: picked }));
  };

  return (
    <>
      <SettingRow
        label={t("delivery.morningLabel")}
        description={t("delivery.morningHelp")}
        testId="delivery-morning"
        control={(control) => (
          <Select
            {...control}
            className="settingrow-measure"
            value={chosen(morning)}
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
            className="settingrow-measure"
            value={chosen(weekly)}
            onChange={pick("weekly_delivery")}
            options={options}
          />
        )}
      />
    </>
  );
}
