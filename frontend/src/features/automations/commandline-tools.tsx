// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import { CLI_PATHS } from "./aiconnect";
import { useConnectorState } from "./connector-state";
import "./connected-agents.css";

// One stacked row per tool: a 60-character command in a right-hand column is
// unreadable.
export function CommandLineToolsCard() {
  const t = useT();
  const connector = useConnectorState();
  return (
    <Panel title={t("aiAccess.cli.title")}>
      <QueryGate query={connector} pendingLabel={t("aiAccess.cli.title")}>
        {(state) =>
          state.enabled ? (
            <>
              <PanelBody>
                <PanelIntro>{t("aiAccess.cli.sub")}</PanelIntro>
              </PanelBody>
              <SettingList bleed="settings">
                {CLI_PATHS.map((path) => (
                  <SettingRow
                    key={path.id}
                    layout="stack"
                    // A product's own name, never translated: it is typed as shown.
                    label={path.name}
                    description={path.note && t(path.note)}
                    control={
                      <div className="agents-guide">
                        <code className="agents-guide-command">
                          {path.command(state.url)}
                        </code>
                        {path.signIn !== undefined && (
                          <span>
                            {t("aiAccess.cli.then", { command: path.signIn })}
                          </span>
                        )}
                        <OffsiteLink href={path.help}>
                          {t("aiAccess.cli.docs", { tool: path.name })}
                        </OffsiteLink>
                      </div>
                    }
                  />
                ))}
              </SettingList>
            </>
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
