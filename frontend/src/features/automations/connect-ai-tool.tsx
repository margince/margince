// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Download } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Button, SegmentedControl } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { downloadBytes } from "../../lib/download";
import {
  AI_PLANS,
  AI_TOOL_NAMES,
  AI_TOOLS,
  type AiPlan,
  type AiTool,
  adminGuide,
  type ConnectPath,
  connectPath,
  STEPS_CHECKED_ON,
} from "./aiconnect";
import { useConnectorState } from "./connector-state";
import "./connect-ai-tool.css";

// The page's first task for a reader who is not a developer: two questions,
// then only the steps for that one tool and plan.
export function ConnectAiToolCard() {
  const t = useT();
  const connector = useConnectorState();
  return (
    <Panel title={t("aiAccess.connect.title")}>
      <PanelBody>
        <PanelIntro>{t("aiAccess.connect.sub")}</PanelIntro>
        <QueryGate query={connector} pendingLabel={t("aiAccess.connect.title")}>
          {(state) =>
            state.enabled ? (
              <Picker mcpUrl={state.url} />
            ) : (
              <Callout title={t("agents.connectorOff")}>
                {t("agents.connectorOffDetail")}
              </Callout>
            )
          }
        </QueryGate>
        <p className="t-caption aiconnect-devhint">
          {t("aiAccess.connect.devHint")}
        </p>
      </PanelBody>
    </Panel>
  );
}

function Picker({ mcpUrl }: Readonly<{ mcpUrl: string }>) {
  const t = useT();
  const [tool, setTool] = useState<AiTool>("claude");
  const [plan, setPlan] = useState<AiPlan>("company");
  return (
    <div className="aiconnect">
      <Question label={t("aiAccess.connect.toolQuestion")}>
        <SegmentedControl
          label={t("aiAccess.connect.toolQuestion")}
          options={AI_TOOLS}
          value={tool}
          onChange={setTool}
          labels={AI_TOOL_NAMES}
        />
      </Question>
      <Question label={t("aiAccess.connect.planQuestion")}>
        <SegmentedControl
          label={t("aiAccess.connect.planQuestion")}
          options={AI_PLANS}
          value={plan}
          onChange={setPlan}
          labels={{
            company: t("aiAccess.plan.company"),
            own: t("aiAccess.plan.own"),
          }}
        />
      </Question>
      <PathBox tool={tool} path={connectPath(tool, plan)} mcpUrl={mcpUrl} />
    </div>
  );
}

function Question({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <div className="aiconnect-question">
      <p className="aiconnect-question-label" aria-hidden="true">
        {label}
      </p>
      {children}
    </div>
  );
}

function PathBox({
  tool,
  path,
  mcpUrl,
}: Readonly<{ tool: AiTool; path: ConnectPath; mcpUrl: string }>) {
  const t = useT();
  const help = t(
    path.helpForAdmin ? "aiAccess.connect.helpAdmin" : "aiAccess.connect.help",
    {
      vendor: path.vendor,
    },
  );
  return (
    <div className="aiconnect-path" data-testid="aiconnect-path">
      {path.unavailable !== undefined && (
        <Callout title={t(path.unavailable)} />
      )}
      {path.admin !== undefined && (
        <AdminRow
          tool={tool}
          path={path}
          mcpUrl={mcpUrl}
          line={path.admin.line}
        />
      )}
      {path.steps.length > 0 && <Steps path={path} mcpUrl={mcpUrl} />}
      {path.note !== undefined && (
        <p className="t-caption aiconnect-indent">{t(path.note)}</p>
      )}
      <p className="aiconnect-indent">
        <OffsiteLink href={path.help}>{help}</OffsiteLink>
      </p>
    </div>
  );
}

// The address sits inside the step that asks for it, so the list stays one list.
function Steps({
  path,
  mcpUrl,
}: Readonly<{ path: ConnectPath; mcpUrl: string }>) {
  const t = useT();
  return (
    <ol className="aiconnect-steps">
      {path.steps.map((key, index) => (
        <li key={key} className="aiconnect-step">
          <div className="aiconnect-step-body">
            {t(key)}
            {index + 1 === path.pasteAfter && <AddressRow mcpUrl={mcpUrl} />}
          </div>
        </li>
      ))}
    </ol>
  );
}

function AdminRow({
  tool,
  path,
  mcpUrl,
  line,
}: Readonly<{
  tool: AiTool;
  path: ConnectPath;
  mcpUrl: string;
  line: MessageKey;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const download = () => {
    const guide = adminGuide({
      tool,
      path,
      mcpUrl,
      t,
      checkedOn: formatDate(
        `${STEPS_CHECKED_ON}T12:00:00Z`,
        locale,
        viewerZone(),
      ),
    });
    downloadBytes(
      guide.markdown,
      guide.filename,
      "text/markdown;charset=utf-8",
    );
  };
  return (
    <div className="aiconnect-admin">
      <div className="aiconnect-admin-text">
        <p>
          <strong>{t("aiAccess.connect.adminFirst")}</strong> {t(line)}
        </p>
        <p className="t-caption">{t("aiAccess.connect.adminSend")}</p>
      </div>
      <Button onClick={download}>
        <Download aria-hidden />
        {t("aiAccess.connect.download")}
      </Button>
    </div>
  );
}

function AddressRow({ mcpUrl }: Readonly<{ mcpUrl: string }>) {
  const t = useT();
  const clipboard = useClipboardCopy(mcpUrl, {
    copy: t("aiAccess.connect.copyAddress"),
    copied: t("aiAccess.connect.copied"),
    remedy: t("aiAccess.connect.copyRemedy"),
  });
  return (
    <div className="aiconnect-address">
      <CopyableText text={mcpUrl} label={t("aiAccess.connect.address")} />
      <Button variant="primary" onClick={clipboard.copy}>
        {clipboard.label}
      </Button>
      {clipboard.notice}
    </div>
  );
}
