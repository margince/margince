// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CircleHelp } from "lucide-react";
import { Callout } from "../../design-system/callout";
import { CopyAction, CopyableText } from "../../design-system/clipboardcopy";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { Popover } from "../../design-system/popover";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { useSaveClientMethods } from "./agent-clients.queries";

function AboutThisExperiment() {
  const t = useT();
  const question = t("agentClients.experiments.clientCheckQuestion");
  return (
    <Popover
      className="agent-experiment-help"
      panelClassName="agent-experiment-panel"
      width="wide"
      label={
        <>
          <CircleHelp aria-hidden />
          <span className="sr-only">{t("agentClients.experiments.about")}</span>
        </>
      }
    >
      <Heading size="xsmall">{t("agentClients.experiments.whyTitle")}</Heading>
      <p>{t("agentClients.experiments.why")}</p>
      <Heading size="xsmall">{t("agentClients.experiments.howTitle")}</Heading>
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
      <Heading size="xsmall">{t("agentClients.experiments.safeTitle")}</Heading>
      <p>{t("agentClients.experiments.safe")}</p>
    </Popover>
  );
}

/**
 * Features still being tried out, off for everyone until an admin turns one
 * on. Today the one experiment is the admin-only `check_client` tool.
 */
export function AgentClientExperimentsCard() {
  const t = useT();
  const canManage = useCanWrite("agent_client", "update");
  const settings = useInstallationSettings(canManage);
  const save = useSaveClientMethods();
  if (!canManage) {
    return null;
  }
  return (
    <Panel title={t("agentClients.experiments.title")}>
      <PanelBody>
        <PanelIntro>{t("agentClients.experiments.sub")}</PanelIntro>
        {save.error && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("agentClients.methods.saveFailed")}
          >
            {problemMessageOf(save.error, t)}
          </Callout>
        )}
      </PanelBody>
      <SettingList bleed="settings">
        <SettingRow
          label={t("agentClients.experiments.clientCheck")}
          description={
            <>
              {t("agentClients.experiments.clientCheckHint")}{" "}
              <AboutThisExperiment />
            </>
          }
          control={
            <Switch
              label={t("agentClients.experiments.clientCheck")}
              labelHidden
              checked={settings.data?.mcp_client_check ?? false}
              pending={save.isPending}
              disabled={settings.data === undefined}
              onChange={(next) => save.mutate({ mcp_client_check: next })}
            />
          }
        />
      </SettingList>
      <PanelNotices readOnly={false} />
    </Panel>
  );
}
