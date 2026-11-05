// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { CopyAction, CopyableText } from "../../design-system/clipboardcopy";
import { describedByIds } from "../../design-system/describedby";
import { Heading } from "../../design-system/heading";
import { HelpPopover } from "../../design-system/helppopover";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useAgentClients, useSaveClientMethods } from "./agent-clients.queries";

function AboutThisExperiment() {
  const t = useT();
  const question = t("agentClients.experiments.clientCheckQuestion");
  return (
    <HelpPopover label={t("agentClients.experiments.about")}>
      <div className="agent-experiment-panel">
        <Heading size="xsmall">
          {t("agentClients.experiments.whyTitle")}
        </Heading>
        <p>{t("agentClients.experiments.why")}</p>
        <Heading size="xsmall">
          {t("agentClients.experiments.howTitle")}
        </Heading>
        <ol className="agent-experiment-steps">
          <li>{t("agentClients.experiments.step1")}</li>
          <li>{t("agentClients.experiments.step2")}</li>
          <li>{t("agentClients.experiments.step3")}</li>
        </ol>
        <CopyableText
          text={question}
          label={t("agentClients.experiments.clientCheckQuestionLabel")}
        />
        <CopyAction
          text={question}
          labels={{
            copy: t("agentClients.experiments.copy"),
            copied: t("agentClients.experiments.copied"),
            remedy: t("agentClients.experiments.copyRemedy"),
          }}
        />
        <p>{t("agentClients.experiments.step4")}</p>
        <Heading size="xsmall">
          {t("agentClients.experiments.safeTitle")}
        </Heading>
        <p>{t("agentClients.experiments.safe")}</p>
      </div>
    </HelpPopover>
  );
}

/**
 * Features still being tried out, off for everyone until an admin turns one
 * on. Today the one experiment is the admin-only `check_client` tool.
 */
export function AgentClientExperimentsCard() {
  const t = useT();
  const canRead = useCan("agent_client", "read");
  // The save is PATCH /installation/settings, which also asks installation_settings update.
  const canSwitch = useCanWrite("agent_client", "update");
  const canSave = useCanWrite("installation_settings", "update");
  const canManage = canSwitch && canSave;
  const clients = useAgentClients(canRead);
  const save = useSaveClientMethods();
  const refusedId = useId();
  if (!canRead) {
    return null;
  }
  const clientCheck = {
    ...clients,
    data: clients.data?.pages[0]?.client_check,
  };
  return (
    <Panel title={t("agentClients.experiments.title")}>
      <PanelBody>
        <PanelIntro>{t("agentClients.experiments.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate
        query={clientCheck}
        pendingLabel={t("agentClients.experiments.title")}
      >
        {(on) => (
          <SettingList bleed="settings">
            <SettingRow
              label={t("agentClients.experiments.clientCheck")}
              description={
                <>
                  {t("agentClients.experiments.clientCheckHint")}{" "}
                  <AboutThisExperiment />
                </>
              }
              control={(control) => (
                <Switch
                  label={t("agentClients.experiments.clientCheck")}
                  labelHidden
                  checked={on}
                  describedBy={describedByIds(
                    control["aria-describedby"],
                    save.isError && refusedId,
                  )}
                  pending={save.isPending}
                  disabled={!canManage}
                  onChange={(next) => save.mutate({ mcp_client_check: next })}
                />
              )}
            />
          </SettingList>
        )}
      </QueryGate>
      <PanelNotices
        readOnly={canManage ? false : t("agentClients.methods.readOnly")}
        refused={
          save.isError
            ? {
                title: t("agentClients.methods.saveFailed"),
                error: save.error,
                id: refusedId,
              }
            : undefined
        }
      />
    </Panel>
  );
}
