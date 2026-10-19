// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { SegmentedControl } from "../../design-system/atoms";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { CLI_NAMES, CLI_PATHS, type CliId } from "./aiconnect";
import { useConnectorState } from "./connector-state";
import "./coding-agents.css";

// One tool at a time, picked the way the main card picks an AI tool. Three
// stacked recipes made the developer section the longest part of the page.
export function CodingAgentsCard() {
  const t = useT();
  const connector = useConnectorState();
  return (
    <Panel title={t("aiAccess.cli.title")}>
      <QueryGate query={connector} pendingLabel={t("aiAccess.cli.title")}>
        {(state) =>
          state.enabled ? (
            <PanelBody>
              <PanelIntro>{t("aiAccess.cli.sub")}</PanelIntro>
              <Recipe mcpUrl={state.url} />
            </PanelBody>
          ) : (
            <SettingList bleed="settings">
              <SettingRow
                label={t("agents.connectorOff")}
                description={t("agents.connectorOffDetail")}
                control={null}
              />
            </SettingList>
          )
        }
      </QueryGate>
    </Panel>
  );
}

function Recipe({ mcpUrl }: Readonly<{ mcpUrl: string }>) {
  const t = useT();
  const [id, setId] = useState<CliId>("claude-code");
  const path = CLI_PATHS.find((candidate) => candidate.id === id);
  if (path === undefined) {
    return null;
  }
  return (
    <div className="agents-guide">
      <SegmentedControl
        label={t("aiAccess.cli.title")}
        options={CLI_PATHS.map((candidate) => candidate.id)}
        value={path.id}
        onChange={setId}
        labels={CLI_NAMES}
      />
      {path.note !== undefined && <p>{t(path.note)}</p>}
      <code className="agents-guide-command">{path.command(mcpUrl)}</code>
      {path.signIn !== undefined && (
        <p>{t("aiAccess.cli.then", { command: path.signIn })}</p>
      )}
      <OffsiteLink href={path.help}>
        {t("aiAccess.cli.docs", { tool: CLI_NAMES[path.id] })}
      </OffsiteLink>
    </div>
  );
}
