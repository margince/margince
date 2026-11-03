// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { describedByIds } from "../../design-system/describedby";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { useMe } from "../../lib/session";
import { useTeams } from "./teams.queries";

type Delivery = components["schemas"]["BriefDelivery"];
const weekdayKeys = [
  "delivery.weekday1",
  "delivery.weekday2",
  "delivery.weekday3",
  "delivery.weekday4",
  "delivery.weekday5",
  "delivery.weekday6",
  "delivery.weekday7",
] as const;
const scopeKeys = {
  personal: "delivery.scope.personal",
  team: "delivery.scope.team",
  combined: "delivery.scope.combined",
} as const;
const scopes = ["personal", "team", "combined"] as const;

export function RoutineDeliveryRows({
  settings,
  save,
  disabled = false,
  refusalFor,
}: Readonly<{
  settings: Delivery;
  save: (patch: Delivery) => void;
  disabled?: boolean;
  refusalFor?: (field: keyof Delivery) => string | undefined;
}>) {
  const t = useT();
  const me = useMe();
  const teams = useTeams();
  // From /me, not the worklist: assembling the whole day to learn one answer
  // made this screen wait on the slowest read in the app.
  const reach = me.data?.team_week ?? "none";
  const selectedScope = settings.routine_mail_scope ?? "personal";
  const teamOptions = (teams.data ?? [])
    .filter(
      (team) => reach === "every_team" || me.data?.teams.includes(team.id),
    )
    .map((team) => ({ value: team.id, label: team.name }));
  return (
    <>
      <SettingRow
        label={t("delivery.hourLabel")}
        testId="delivery-time"
        description={t("delivery.scheduleHelp")}
        control={(control) => (
          <Select
            {...control}
            disabled={disabled}
            aria-describedby={describedByIds(
              control["aria-describedby"],
              refusalFor?.("routine_delivery_time"),
            )}
            value={
              settings.routine_delivery_time ??
              (settings.delivery_hour_local === undefined
                ? "working"
                : "legacy")
            }
            options={[
              ...(settings.routine_delivery_time === undefined &&
              settings.delivery_hour_local !== undefined
                ? [
                    {
                      value: "legacy",
                      label: `${t("delivery.legacyTime")} ${String(settings.delivery_hour_local).padStart(2, "0")}:00`,
                    },
                  ]
                : []),
              { value: "working", label: t("delivery.workingStart") },
              ...Array.from({ length: 24 }, (_, hour) => ({
                value: `${String(hour).padStart(2, "0")}:00`,
                label: `${String(hour).padStart(2, "0")}:00`,
              })),
            ]}
            onChange={(value) => {
              if (
                value === "working" ||
                /^(?:[01][0-9]|2[0-3]):00$/.test(value)
              )
                save({ routine_delivery_time: value });
            }}
          />
        )}
      />
      <SettingRow
        label={t("delivery.weekdayLabel")}
        testId="delivery-weekday"
        description={t("delivery.weekdayHelp")}
        control={(control) => (
          <Select
            {...control}
            disabled={disabled}
            aria-describedby={describedByIds(
              control["aria-describedby"],
              refusalFor?.("weekly_delivery_day"),
            )}
            value={settings.weekly_delivery_day?.toString() ?? "1"}
            options={Array.from({ length: 7 }, (_, index) => ({
              value: String(index + 1),
              label: t(weekdayKeys[index] ?? "delivery.weekday1"),
            }))}
            onChange={(value) => {
              const picked = Number(value);
              if (picked >= 1 && picked <= 7)
                save({ weekly_delivery_day: picked });
            }}
          />
        )}
      />
      <SettingRow
        label={t("delivery.quietLabel")}
        testId="delivery-quiet"
        control={(control) => (
          <Select
            {...control}
            disabled={disabled}
            aria-describedby={describedByIds(
              control["aria-describedby"],
              refusalFor?.("quiet_day_notice"),
            )}
            value={settings.quiet_day_notice ? "yes" : "no"}
            options={[
              { value: "no", label: t("delivery.onlyUseful") },
              { value: "yes", label: t("delivery.quietAlso") },
            ]}
            onChange={(value) => save({ quiet_day_notice: value === "yes" })}
          />
        )}
      />
      <SettingRow
        label={t("delivery.combineLabel")}
        testId="delivery-combine"
        description={t("delivery.combineHelp")}
        control={(control) => (
          <Select
            {...control}
            disabled={disabled}
            aria-describedby={describedByIds(
              control["aria-describedby"],
              refusalFor?.("combine_routine_email"),
            )}
            value={settings.combine_routine_email ? "yes" : "no"}
            options={[
              { value: "no", label: t("delivery.separate") },
              { value: "yes", label: t("delivery.combined") },
            ]}
            onChange={(value) =>
              save({ combine_routine_email: value === "yes" })
            }
          />
        )}
      />
      {(reach !== "none" || selectedScope !== "personal") && (
        <>
          <SettingRow
            label={t("delivery.scopeLabel")}
            testId="delivery-scope"
            control={(control) => (
              <Select
                {...control}
                disabled={disabled}
                aria-describedby={describedByIds(
                  control["aria-describedby"],
                  refusalFor?.("routine_mail_scope"),
                )}
                value={selectedScope}
                options={scopes
                  .filter(
                    (scope) =>
                      scope === "personal" || settings.routine_mail_team_id,
                  )
                  .map((scope) => ({
                    value: scope,
                    label: t(scopeKeys[scope]),
                  }))}
                onChange={(value) => {
                  const scope = scopes.find((candidate) => candidate === value);
                  if (
                    scope &&
                    (scope === "personal" || settings.routine_mail_team_id)
                  )
                    save({ routine_mail_scope: scope });
                }}
              />
            )}
          />
          <SettingRow
            label={t("teamweekly.pickTeam")}
            testId="delivery-team"
            description={t("delivery.teamHelp")}
            control={(control) =>
              // A team list still loading, or one that failed, is not "no teams":
              // an empty picker would tell a lead they lead nobody.
              teams.isSuccess ? (
                <Select
                  {...control}
                  disabled={disabled}
                  aria-describedby={describedByIds(
                    control["aria-describedby"],
                    refusalFor?.("routine_mail_team_id"),
                  )}
                  value={settings.routine_mail_team_id ?? ""}
                  placeholder={t("teamweekly.pickTeam")}
                  options={teamOptions}
                  onChange={(value) => {
                    if (teamOptions.some((team) => team.value === value))
                      save({ routine_mail_team_id: value });
                  }}
                />
              ) : (
                <QueryStates
                  query={teams}
                  pendingLabel={t("teamweekly.pickTeam")}
                  pendingLines={1}
                >
                  {null}
                </QueryStates>
              )
            }
          />
        </>
      )}
    </>
  );
}
