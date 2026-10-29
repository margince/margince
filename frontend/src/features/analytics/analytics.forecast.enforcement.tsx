import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useHoldsAdminRole } from "../../lib/capability";
import { unwrap } from "../../lib/problem";

const enforcementKey = ["forecast-assurance-enforcement"];

// Whether the qualified forecast leaves out deals under an open review.
//
// It sits with the check it depends on. The switch is refused until a first
// check has run: before that no deal has been judged, and the qualified
// forecast would count nothing. Every reader sees the state; only an admin
// flips it.
export function EnforcementSwitch() {
  const t = useT();
  const toast = useToast();
  const client = useQueryClient();
  const isAdmin = useHoldsAdminRole();
  const state = useQuery({
    queryKey: enforcementKey,
    queryFn: async () =>
      unwrap(await api.GET("/forecast/assurance/enforcement", {})).state,
  });
  const flip = useMutation({
    mutationFn: async (enabled: boolean) =>
      unwrap(
        await api.PUT("/forecast/assurance/enforcement", {
          body: { enabled },
        }),
      ).state,
    onSuccess: async (next) => {
      client.setQueryData(enforcementKey, next);
      toast.show(
        next === "enforcing"
          ? t("review.enforcementOn")
          : t("review.enforcementOff"),
      );
      // Every forecast reading carries the qualified figures or not, so all of
      // them are stale the moment this changes.
      await client.invalidateQueries({ queryKey: ["forecast"] });
    },
  });
  if (!state.data) {
    return null;
  }
  const reason = !isAdmin
    ? t("review.enforcementAdminOnly")
    : state.data === "not_enrolled"
      ? t("review.enforcementNotStarted")
      : undefined;
  return (
    <SettingList>
      <SettingRow
        label={t("review.enforcementLabel")}
        description={t("review.enforcementHint")}
        control={
          <Switch
            label={t("review.enforcementLabel")}
            labelHidden
            checked={state.data === "enforcing"}
            pending={flip.isPending}
            reason={reason}
            onChange={(next) => flip.mutate(next)}
          />
        }
      />
    </SettingList>
  );
}
