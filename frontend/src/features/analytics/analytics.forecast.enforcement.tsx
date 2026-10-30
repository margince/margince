import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useHoldsAdminRole } from "../../lib/capability";
import { unwrap } from "../../lib/problem";

const enforcementKey = ["forecast-assurance-enforcement"];

// Whether the qualified forecast leaves out deals under an open review.
//
// It sits with the check it depends on. The switch is refused until a first
// check has run: before that no deal has been judged, and the qualified
// forecast would count nothing. Every reader sees the state; only an admin
// flips it, after seeing what it would leave out. The switch sends back the
// preview it showed, and a preview that changed since is refused and shown anew.
export function EnforcementSwitch() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const client = useQueryClient();
  const isAdmin = useHoldsAdminRole();
  const state = useQuery({
    queryKey: enforcementKey,
    queryFn: async () =>
      unwrap(await api.GET("/forecast/assurance/enforcement", {})),
  });
  const preview = state.data?.preview;
  const flip = useMutation({
    // The digest rides with the click, so it is the one on screen when the
    // admin pressed, not one an earlier render captured.
    mutationFn: async ({
      enabled,
      digest,
    }: {
      enabled: boolean;
      digest?: string;
    }) =>
      unwrap(
        await api.PUT("/forecast/assurance/enforcement", {
          body: { enabled, preview_digest: digest },
        }),
      ),
    onError: async () => {
      await client.invalidateQueries({ queryKey: enforcementKey });
    },
    onSuccess: async (next) => {
      client.setQueryData(enforcementKey, next);
      toast.show(
        next.state === "enforcing"
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
    : state.data.state === "not_enrolled"
      ? t("review.enforcementNotStarted")
      : undefined;
  return (
    <SettingList>
      <SettingRow
        label={t("review.enforcementLabel")}
        description={[
          t("review.enforcementHint"),
          preview &&
            plural("review.enforcementPreviewLeftOut", preview.left_out_count, {
              count: formatNumber(preview.left_out_count, locale),
            }),
          preview &&
            preview.unknown_count > 0 &&
            plural("review.enforcementPreviewUnknown", preview.unknown_count, {
              count: formatNumber(preview.unknown_count, locale),
            }),
        ]
          .filter(Boolean)
          .join(" ")}
        control={
          <Switch
            label={t("review.enforcementLabel")}
            labelHidden
            checked={state.data.state === "enforcing"}
            pending={flip.isPending}
            reason={reason}
            onChange={(next) =>
              flip.mutate({ enabled: next, digest: preview?.digest })
            }
          />
        }
      />
    </SettingList>
  );
}
