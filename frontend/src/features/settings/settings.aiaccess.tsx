// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { ConnectAiToolCard } from "../../components/connect-ai-tool";
import { SectionHeader, SegmentedControl } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { PanelTitleLevel } from "../../design-system/panel";
import { useT } from "../../i18n";
import { AGENT_RULES_SUBPAGE } from "../../lib/settingsrouting";
import { ApprovalPreferencesCard } from "../automations/approval-preferences";
import { ConnectedAgentsCard } from "../automations/connected-agents";
import { AutonomySettingsCard } from "./autonomy-settings";
import { AgentToolsCard } from "./settings-agenttools";
import { AutonomyTiersCard } from "./settings-autonomytiers";
import { PassportCard } from "./settings-passports";
import "./settings-aiaccess.css";

// Every seat gets this page, ungated: a connection's authority is the reader's
// own consent. Connecting a tool comes first, then the reader's own agent
// switches; passports and agent rules wait behind "For developers".
export function AiAccessTab({ subpage }: Readonly<{ subpage?: string }>) {
  const t = useT();
  return (
    <>
      <ConnectAiToolCard />
      <ConnectedAgentsCard />
      <SectionHeader title={t("aiAccess.yourAgent.title")} />
      <PanelTitleLevel value={3}>
        <AutonomySettingsCard />
        <ApprovalPreferencesCard />
      </PanelTitleLevel>
      <DeveloperSection onRules={subpage === AGENT_RULES_SUBPAGE} />
    </>
  );
}

const DEVELOPER_TABS = ["passports", "rules"] as const;
type DeveloperTab = (typeof DEVELOPER_TABS)[number];

// Closed until asked for, or until the address names Agent rules. Passports
// open first: a developer who opens this section most often came for one.
function DeveloperSection({ onRules }: Readonly<{ onRules: boolean }>) {
  const t = useT();
  const [tab, setTab] = useState<DeveloperTab>(onRules ? "rules" : "passports");
  const section = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (onRules) section.current?.scrollIntoView({ block: "start" });
  }, [onRules]);
  return (
    <div ref={section}>
      <Disclosure
        open={onRules}
        summary={
          <span className="aiaccess-dev-title">{t("aiAccess.dev.title")}</span>
        }
      >
        <div className="aiaccess-dev">
          <p className="t-caption aiaccess-dev-sub">{t("aiAccess.dev.sub")}</p>
          {/* Not written to the address: every settings address remounts
              the page, which drops focus and this section's state. */}
          <SegmentedControl
            label={t("aiAccess.dev.title")}
            options={DEVELOPER_TABS}
            value={tab}
            onChange={setTab}
            labels={{
              passports: t("aiAccess.dev.tab.passports"),
              rules: t("aiAccess.dev.tab.rules"),
            }}
          />
          {/* Both stay mounted, so a tab keeps its state and settings search
              finds every card; only the chosen one shows. */}
          <div hidden={tab !== "passports"}>
            <PassportCard />
          </div>
          <div hidden={tab !== "rules"} className="aiaccess-dev-rules">
            <AgentToolsCard />
            <AutonomyTiersCard />
          </div>
        </div>
      </Disclosure>
    </div>
  );
}
