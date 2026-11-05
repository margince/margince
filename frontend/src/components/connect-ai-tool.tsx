// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Download } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Button, SegmentedControl } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { Field } from "../design-system/field";
import { OffsiteLink } from "../design-system/offsitelink";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  AI_CHOICE_NAMES,
  AI_CHOICES,
  AI_PLACES,
  AI_PLANS,
  type AiChoice,
  type AiPlace,
  type AiPlan,
  type AiTool,
  adminGuide,
  CLI_NAMES,
  type CliPath,
  type ConnectPath,
  cliPath,
  connectPath,
  STEPS_CHECKED_ON,
} from "../lib/aiconnect";
import { useConnectorState } from "../lib/connector-state";
import { downloadBytes } from "../lib/download";
import { useViewerZone } from "../lib/viewerzone";
import "./connect-ai-tool.css";

// The page's first task for a reader who is not a developer. It asks which AI
// and where it is used, then shows only that pair's steps.
export function ConnectAiToolCard() {
  const t = useT();
  return (
    <Panel title={t("aiAccess.connect.title")}>
      <PanelBody>
        <PanelIntro>{t("aiAccess.connect.sub")}</PanelIntro>
        <ConnectAiTool />
        <p className="t-caption aiconnect-devhint">
          {t("aiAccess.connect.devHint")}
        </p>
      </PanelBody>
    </Panel>
  );
}

/** The picker and its steps without a card, for a dialog that has its own. */
export function ConnectAiTool() {
  const t = useT();
  const connector = useConnectorState();
  return (
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
  );
}

function Picker({ mcpUrl }: Readonly<{ mcpUrl: string }>) {
  const t = useT();
  const [choice, setChoice] = useState<AiChoice>("claude");
  const [placeId, setPlaceId] = useState(AI_PLACES.claude[0]?.id ?? "");
  const places = AI_PLACES[choice];
  const place = places.find((candidate) => candidate.id === placeId);
  const choiceName = (key: AiChoice) =>
    key === "other" ? t("aiAccess.choice.other") : AI_CHOICE_NAMES[key];
  return (
    <div className="aiconnect">
      <div className="aiconnect-pickers">
        <Field label={t("aiAccess.connect.toolQuestion")}>
          {(control) => (
            <Select
              {...control}
              options={AI_CHOICES.map((key) => ({
                value: key,
                label: choiceName(key),
              }))}
              value={choice}
              onChange={(next) => {
                const picked = AI_CHOICES.find((key) => key === next);
                if (picked) {
                  setChoice(picked);
                  setPlaceId(AI_PLACES[picked][0]?.id ?? "");
                }
              }}
            />
          )}
        </Field>
        {/* An AI used in one place only is not asked where. */}
        {places.length > 1 && (
          <Field label={t("aiAccess.connect.placeQuestion")}>
            {(control) => (
              <Select
                {...control}
                options={places.map((candidate) => ({
                  value: candidate.id,
                  label: candidate.label ? t(candidate.label) : candidate.id,
                }))}
                value={placeId}
                onChange={setPlaceId}
              />
            )}
          </Field>
        )}
      </div>
      {place === undefined ? (
        <OtherPath mcpUrl={mcpUrl} />
      ) : (
        <PlacePath
          key={`${choice}-${place.id}`}
          place={place}
          mcpUrl={mcpUrl}
        />
      )}
    </div>
  );
}

function PlacePath({
  place,
  mcpUrl,
}: Readonly<{ place: AiPlace; mcpUrl: string }>) {
  const t = useT();
  const { route } = place;
  return (
    <>
      {route.kind === "app" ? (
        <AppPath tool={route.tool} mcpUrl={mcpUrl} />
      ) : (
        <LocalPath
          path={cliPath(route.cli)}
          runIn={route.runIn}
          mcpUrl={mcpUrl}
        />
      )}
      {place.note !== undefined && <p className="t-caption">{t(place.note)}</p>}
    </>
  );
}

// Only a chat app asks whose plan it is: there an admin may have to act first.
function AppPath({ tool, mcpUrl }: Readonly<{ tool: AiTool; mcpUrl: string }>) {
  const t = useT();
  const [plan, setPlan] = useState<AiPlan>("company");
  return (
    <>
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
    </>
  );
}

function LocalPath({
  path,
  runIn,
  mcpUrl,
}: Readonly<{ path: CliPath; runIn?: MessageKey; mcpUrl: string }>) {
  const t = useT();
  return (
    <div className="aiconnect-path" data-testid="aiconnect-path">
      <p className="t-caption">{t("aiAccess.local.sub")}</p>
      {runIn !== undefined && <p>{t(runIn)}</p>}
      <ol className="aiconnect-steps">
        {path.steps.map((step) => (
          <li key={step.text} className="aiconnect-step">
            <div className="aiconnect-step-body">
              {t(step.text)}
              {step.copy === "address" && <AddressRow mcpUrl={mcpUrl} />}
              {typeof step.copy === "function" && (
                <CopyRow text={step.copy(mcpUrl)} />
              )}
            </div>
          </li>
        ))}
      </ol>
      <p className="aiconnect-indent">
        <OffsiteLink href={path.help}>
          {t("aiAccess.cli.docs", { tool: CLI_NAMES[path.id] })}
        </OffsiteLink>
      </p>
    </div>
  );
}

// Any tool that adds a remote MCP server gets the three facts each one asks for.
// Two captions cover a tool that cannot use one of them.
function OtherPath({ mcpUrl }: Readonly<{ mcpUrl: string }>) {
  const t = useT();
  return (
    <div className="aiconnect-path" data-testid="aiconnect-path">
      <p>{t("aiAccess.other.sub")}</p>
      <dl className="aiconnect-facts">
        <dt>{t("aiAccess.connect.address")}</dt>
        <dd>
          <AddressRow mcpUrl={mcpUrl} />
        </dd>
        <dt>{t("aiAccess.other.typeLabel")}</dt>
        <dd>{t("aiAccess.other.type")}</dd>
        <dt>{t("aiAccess.other.signInLabel")}</dt>
        <dd>{t("aiAccess.other.signIn")}</dd>
      </dl>
      <p className="t-caption">{t("aiAccess.other.apiKey")}</p>
      <p className="t-caption">{t("aiAccess.other.notAllowed")}</p>
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
      {/* A path that cannot connect has no vendor steps worth following. */}
      {path.unavailable === undefined && (
        <p className="aiconnect-indent">
          <OffsiteLink href={path.help}>{help}</OffsiteLink>
        </p>
      )}
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
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const download = () => {
    const guide = adminGuide({
      tool,
      path,
      mcpUrl,
      t,
      checkedOn: formatDate(STEPS_CHECKED_ON, locale, readerZone),
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

function CopyRow({ text }: Readonly<{ text: string }>) {
  const t = useT();
  const clipboard = useClipboardCopy(text, {
    copy: t("aiAccess.connect.copy"),
    copied: t("aiAccess.connect.copied"),
    remedy: t("aiAccess.connect.copyRemedy"),
  });
  return (
    <div className="aiconnect-address">
      <CopyableText text={text} label={t("aiAccess.connect.command")} />
      <Button onClick={clipboard.copy}>{clipboard.label}</Button>
      {clipboard.notice}
    </div>
  );
}
