// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Automation = components["schemas"]["Automation"];

const STATE_LABEL: Readonly<Record<string, MessageKey>> = {
  needs_setup: "auto.setup.needsSetup",
  needs_attention: "auto.setup.needsAttention",
  on: "auto.setup.on",
  paused: "auto.statusPaused",
};
const REASON_LABEL: Readonly<Record<string, MessageKey>> = {
  configuration_incomplete: "auto.setup.incomplete",
  authorizer_missing: "auto.setup.noAuthorizer",
  authorizer_inactive: "auto.setup.inactive",
};

export function AutomationSetup({
  automation,
}: Readonly<{ automation: Automation }>) {
  const t = useT();
  const state = automation.setup_state;
  if (!state) return null;
  const fallback =
    automation.status === "enabled"
      ? "auto.statusEnabled"
      : "auto.statusPaused";
  const label = STATE_LABEL[state] ?? fallback;
  const detail = automation.setup_reason
    ? REASON_LABEL[automation.setup_reason]
    : undefined;
  const tone =
    state === "on"
      ? "success"
      : state === "needs_attention"
        ? "danger"
        : "warning";
  return (
    <>
      <Badge tone={tone}>{t(label)}</Badge>
      {detail && <p className="t-caption">{t(detail)}</p>}
    </>
  );
}
