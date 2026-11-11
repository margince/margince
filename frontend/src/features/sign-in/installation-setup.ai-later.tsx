// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useInstallationSetup } from "../../lib/installation-setup-query";
import { AiStep } from "./installation-setup.ai-step";

const ignoreBusy = () => undefined;

/**
 * The first-run AI form, offered in Settings while the installation binds no
 * model: the way back for an admin who skipped it on the cold start. It is the
 * same form rather than the Providers and Tiers cards, because those edit a
 * binding that exists and a first one needs a key and a binding together.
 */
export function AiSetupPanel() {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const canBind = useCanWrite("ai_routing", "update");
  const setup = useInstallationSetup();
  const unbound = setup.data?.steps?.some(
    (step) => step.step === "ai_models" && !step.configured,
  );
  if (!canBind || unbound !== true) {
    return null;
  }
  return (
    <AiStep
      panel={{ title: t("aiSetup.title"), intro: t("aiSetup.intro") }}
      verb="aiSetup.connect"
      onBusy={ignoreBusy}
      onIgnite={async (vendor) => {
        toast.show(t("aiSetup.connected", { vendor }));
        // Every surface that read "no model" reads again: the checklist, the
        // provider and tier cards, and this panel, which then steps aside.
        await Promise.all(
          [
            ["installation-setup"],
            ["ai-routing"],
            ["ai-profile"],
            ["ai-status"],
          ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
        );
      }}
    />
  );
}
