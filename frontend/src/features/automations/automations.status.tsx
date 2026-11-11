import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { ErrorLine } from "../../design-system/errorline";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { RulePausedReason, RuleSetByAgent } from "./automations.lists";
import { patchAutomation } from "./automations.patch";
import { ScheduledPopulationReview } from "./automations.populationreview";
import { AutomationSetup } from "./automations.setup";

type Automation = components["schemas"]["Automation"];

// Flipping the switch is the write; a seat without update reads a badge.
export function AutomationStatus({
  automation,
  canEdit,
}: Readonly<{ automation: Automation; canEdit: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [reviewing, setReviewing] = useState(false);
  const flip = useMutation({
    mutationFn: patchAutomation,
    onSuccess: () => setReviewing(false),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: ["automations"] }),
  });
  const enabled = automation.status === "enabled";
  return (
    <CellStack>
      {canEdit ? (
        <Switch
          label={t("auto.enabledFor", { name: automation.name })}
          labelHidden
          checked={enabled}
          pending={flip.isPending}
          onChange={(next) => {
            if (next && automation.initial_run_preview_required) {
              flip.reset();
              setReviewing(true);
            } else {
              flip.mutate({
                id: automation.id,
                version: automation.version,
                body: { status: next ? "enabled" : "paused" },
              });
            }
          }}
        />
      ) : !automation.setup_state ? (
        <Badge tone={enabled ? "success" : "warning"}>
          {enabled ? t("auto.statusEnabled") : t("auto.statusPaused")}
        </Badge>
      ) : null}
      {reviewing && canEdit && (
        <ScheduledPopulationReview
          automation={automation}
          pending={flip.isPending}
          error={flip.isError ? problemMessageOf(flip.error, t) : null}
          onClose={() => setReviewing(false)}
          onConfirm={() =>
            flip.mutate({
              id: automation.id,
              version: automation.version,
              body: { status: "enabled", initial_run_confirmed: true },
            })
          }
        />
      )}
      <AutomationSetup automation={automation} />
      <RuleSetByAgent automation={automation} />
      <RulePausedReason automation={automation} />
      {/* A refused flip moves nothing on screen, so this line is its only report. */}
      {flip.isError && <ErrorLine>{problemMessageOf(flip.error, t)}</ErrorLine>}
    </CellStack>
  );
}
