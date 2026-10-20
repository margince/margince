// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { SegmentedControl } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { useT } from "../../i18n";
import { ApprovalPreferencesCard } from "../automations/approval-preferences";
import { AutonomySettingsCard } from "../automations/autonomy-settings";
import { CodingAgentsCard } from "../automations/coding-agents";
import { ConnectAiToolCard } from "../automations/connect-ai-tool";
import { ConnectedAgentsCard } from "../automations/connected-agents";
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
          <span className="aiaccess-dev-title">{t("aiAccess.dev.title")}</span>
        }
      >
        <DeveloperTabs />
      </Disclosure>
    </>
  );
}

const DEVELOPER_TABS = ["passports", "agents", "rules"] as const;
type DeveloperTab = (typeof DEVELOPER_TABS)[number];

// API keys open first: a developer who opens this section most often came for
// a passport. One tab shows at a time, so the section stays one screen long.
function DeveloperTabs() {
  const t = useT();
  const [tab, setTab] = useState<DeveloperTab>("passports");
  return (
    <div className="aiaccess-dev">
      <p className="t-caption aiaccess-dev-sub">{t("aiAccess.dev.sub")}</p>
      <SegmentedControl
        label={t("aiAccess.dev.title")}
        options={DEVELOPER_TABS}
        value={tab}
        onChange={setTab}
        labels={{
          passports: t("aiAccess.dev.tab.passports"),
          agents: t("aiAccess.dev.tab.agents"),
          rules: t("aiAccess.dev.tab.rules"),
        }}
      />
      {/* All three stay mounted, so a tab keeps its state and settings search
          finds every card; only the chosen one shows. */}
      <div hidden={tab !== "passports"}>
        <PassportCard />
      </div>
      <div hidden={tab !== "agents"}>
        <CodingAgentsCard />
      </div>
      <div hidden={tab !== "rules"} className="aiaccess-dev-rules">
        <AgentToolsCard />
        <AutonomyTiersCard />
        <AutonomySettingsCard />
        <ApprovalPreferencesCard />
      </div>
    </div>
  );
}
