// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Disclosure } from "../design-system/disclosure";
import { AutonomySettingsCard } from "../features/automations/autonomy-settings";
import { CommandLineToolsCard } from "../features/automations/commandline-tools";
import { ConnectAiToolCard } from "../features/automations/connect-ai-tool";
import { ConnectedAgentsCard } from "../features/automations/connected-agents";
import { useT } from "../i18n";
import { AgentToolsCard } from "./settings-agenttools";
import { AutonomyTiersCard } from "./settings-autonomytiers";
import { PassportCard } from "./settings-passports";
import "./settings-aiaccess.css";

// Every seat gets this page, ungated: a connection's authority is the reader's
// own consent. Connecting a tool comes first; tokens and commands wait behind
// "For developers", closed until a reader asks for them.
export function AiAccessTab() {
  const t = useT();
  return (
    <>
      <ConnectAiToolCard />
      <ConnectedAgentsCard />
      <Disclosure
        summary={
          <span className="aiaccess-dev-summary">
            <span className="aiaccess-dev-title">
              {t("aiAccess.dev.title")}
            </span>
            <span className="t-caption">{t("aiAccess.dev.sub")}</span>
          </span>
        }
      >
        <div className="aiaccess-dev">
          <CommandLineToolsCard />
          <PassportCard />
          <AgentToolsCard />
          <AutonomyTiersCard />
          <AutonomySettingsCard />
        </div>
      </Disclosure>
    </>
  );
}
