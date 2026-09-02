// What the product may send this reader about their day and their week.
//
// The brief and the weekly are on their screen either way; these rows are about
// the NUDGE toward them, which is the one part a person is entitled to switch
// off. So they sit under the account's own settings beside the display
// language, not under anything an admin configures for the team.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Select } from "../design-system/select";
import { SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { throwProblem } from "./common";

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
 * it — and touching the control is what turns a default into a decision.
 */
export function BriefDeliveryRows() {
  const t = useT();
  const queryClient = useQueryClient();
  const settings = useQuery({
    queryKey: ["me", "brief-delivery"],
    queryFn: async () => {
      const { data, error } = await api.GET("/me/brief-delivery");
      if (error) {
        throwProblem(error);
      }
      return data ?? {};
    },
  });
  // Each control sends only ITSELF. The endpoint is a patch, so a row that sent
  // the whole form would let a stale render of one dropdown overwrite a choice
  // the reader had just made in another.
  const save = useMutation({
    mutationFn: async (patch: BriefDelivery) => {
      const { error } = await api.PUT("/me/brief-delivery", { body: patch });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["me", "brief-delivery"],
      });
    },
  });

  const chosen = (value: Choice | undefined): Choice => value ?? "email";
  const options = CHOICES.map((choice) => ({
    value: choice,
    label: t(choice === "email" ? "delivery.byEmail" : "delivery.none"),
  }));

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
    </>
  );
}
