// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useId, useState } from "react";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { TokenInput } from "../../design-system/tokeninput";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemFieldErrorsOf } from "../../lib/problem";
import {
  type RegisteredAgentClient,
  useRegisterAgentClient,
} from "./agent-clients.queries";
import { useConnectorState } from "./connector-state";
import "./agent-clients.css";

const NAME_MAX = 120;

// Callbacks of tools that can send a client secret, so an admin picks rather
// than types. The product names are the vendors' own, in every language.
const REDIRECT_PRESETS = [
  { value: "https://claude.ai/api/mcp/auth_callback", hint: "Claude" },
  { value: "http://localhost/callback", hint: "Claude Code" },
  {
    value: "https://chatgpt.com/connector_platform_oauth_redirect",
    hint: "ChatGPT",
  },
  {
    value: "https://vertexaisearch.cloud.google.com/oauth-redirect",
    hint: "Gemini Enterprise",
  },
] as const;

// Server codes from the register call, worded for the field they land on.
const FIELD_PROBLEM_COPY: Readonly<Record<string, MessageKey>> = {
  invalid_name: "agentClients.register.nameInvalid",
  invalid_redirect_uri: "agentClients.register.redirectInvalid",
  redirect_uri_count: "agentClients.register.redirectCount",
};

function useFieldProblem(error: unknown) {
  const t = useT();
  const problems = problemFieldErrorsOf(error);
  return (field: string): string | undefined => {
    const problem = problems.find((entry) => entry.field === field);
    if (problem === undefined) {
      return undefined;
    }
    const key = FIELD_PROBLEM_COPY[problem.code];
    return key === undefined ? problem.message : t(key);
  };
}

/**
 * Registering a client by hand, for a tool that cannot register itself. The
 * dialog then shows what to paste into the tool, and the secret only once.
 * The caller unmounts it on close, so nothing it showed outlives it.
 */
export function RegisterAgentClientDialog({
  onClose,
}: Readonly<{ onClose: () => void }>) {
  const [registered, setRegistered] = useState<RegisteredAgentClient | null>(
    null,
  );
  if (registered !== null) {
    return (
      <AgentClientValuesDialog
        titleKey="agentClients.result.title"
        clientId={registered.client.client_id}
        secret={registered.client_secret}
        scopes={registered.scopes}
        onClose={onClose}
      />
    );
  }
  return <RegisterForm onClose={onClose} onRegistered={setRegistered} />;
}

function RegisterForm({
  onClose,
  onRegistered,
}: Readonly<{
  onClose: () => void;
  onRegistered: (registered: RegisteredAgentClient) => void;
}>) {
  const t = useT();
  const headingId = useId();
  const formId = useId();
  const register = useRegisterAgentClient();
  const fieldProblem = useFieldProblem(register.error);
  const [name, setName] = useState("");
  const [redirects, setRedirects] = useState<readonly string[]>([]);
  const nameProblem = fieldProblem("name");
  const redirectProblem = fieldProblem("redirect_uris");
  const otherProblem =
    register.error && !nameProblem && !redirectProblem ? register.error : null;
  const missing = missingAnswer(name, redirects);
  // An edit answers the server's refusal, so the refusal stops showing.
  function edited<T>(set: (value: T) => void) {
    return (value: T) => {
      if (register.error) {
        register.reset();
      }
      set(value);
    };
  }

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (register.isPending || missing !== undefined) {
      return;
    }
    register.mutate(
      { name: name.trim(), redirect_uris: [...redirects] },
      { onSuccess: onRegistered },
    );
  };

  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={headingId}
      intent="form"
      closeDisabled={register.isPending}
    >
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("agentClients.register.title")}
      </Heading>
      <form id={formId} className="form-stack" onSubmit={submit} noValidate>
        <Field
          label={t("agentClients.register.name")}
          hint={t("agentClients.register.nameHint")}
          error={nameProblem}
          required
        >
          {(control) => (
            <TextInput
              {...control}
              value={name}
              maxLength={NAME_MAX}
              onChange={(event) => edited(setName)(event.target.value)}
            />
          )}
        </Field>
        <Field
          label={t("agentClients.register.redirects")}
          hint={t("agentClients.register.redirectsHint")}
          error={redirectProblem}
          required
        >
          {(control) => (
            <TokenInput
              {...control}
              values={redirects}
              onChange={edited(setRedirects)}
              suggestions={REDIRECT_PRESETS}
            />
          )}
        </Field>
        <ErrorLine error={otherProblem} />
      </form>
      <div className="actions">
        <Button onClick={onClose} disabled={register.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          pending={register.isPending}
          reason={missing === undefined ? undefined : t(missing)}
        >
          {t("agentClients.register.submit")}
        </Button>
      </div>
    </Modal>
  );
}

function missingAnswer(
  name: string,
  redirects: readonly string[],
): MessageKey | undefined {
  if (name.trim() === "") {
    return "agentClients.register.needName";
  }
  if (redirects.length === 0) {
    return "agentClients.register.needRedirect";
  }
  return undefined;
}

/**
 * What a tool's admin pastes in, the secret included. Shown after a register
 * and after a rotate; the secret lives only in the caller's state, which ends
 * when the dialog closes.
 */
export function AgentClientValuesDialog({
  titleKey,
  clientId,
  secret,
  scopes,
  onClose,
}: Readonly<{
  titleKey: MessageKey;
  clientId: string;
  secret: string;
  scopes: string;
  onClose: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  const connector = useConnectorState();
  const mcpUrl = connector.data?.enabled ? connector.data.url : null;
  const issuer = mcpUrl === null ? null : new URL(mcpUrl).origin;
  // The clipboard is one slot, so only the last copy says Copied.
  const [current, setCurrent] = useState<MessageKey | null>(null);
  const values: { label: MessageKey; text: string }[] = [
    { label: "agentClients.value.clientId", text: clientId },
    { label: "agentClients.value.secret", text: secret },
    ...(issuer === null || mcpUrl === null
      ? []
      : [
          {
            label: "agentClients.value.authorize" as const,
            text: `${issuer}/oauth/authorize`,
          },
          {
            label: "agentClients.value.token" as const,
            text: `${issuer}/oauth/token`,
          },
          {
            label: "agentClients.value.scopes" as const,
            text: scopes,
          },
          { label: "agentClients.value.mcpUrl" as const, text: mcpUrl },
        ]),
  ];
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t(titleKey)}
      </Heading>
      <div className="form-stack">
        <Callout
          tone="warning"
          kind="standing"
          title={t("agentClients.result.secretOnceTitle")}
        >
          {t("agentClients.result.secretOnceBody")}
        </Callout>
        {values.map((value) => (
          <CopyValue
            key={value.label}
            label={t(value.label)}
            text={value.text}
            current={current === value.label}
            onCopied={() => setCurrent(value.label)}
          />
        ))}
        {connector.data?.enabled === false && (
          <Callout title={t("agents.connectorOff")}>
            {t("agents.connectorOffDetail")}
          </Callout>
        )}
        {mcpUrl !== null && <GeminiSteps />}
      </div>
      <div className="actions">
        <Button variant="primary" onClick={onClose}>
          {t("agentClients.result.done")}
        </Button>
      </div>
    </Modal>
  );
}

function CopyValue({
  label,
  text,
  current,
  onCopied,
}: Readonly<{
  label: string;
  text: string;
  current: boolean;
  onCopied: () => void;
}>) {
  const t = useT();
  const copy = useClipboardCopy(
    text,
    {
      copy: t("agentClients.value.copy"),
      copied: t("agentClients.value.copied"),
      remedy: t("agentClients.value.copyRemedy"),
    },
    onCopied,
  );
  const shown = current && copy.copied;
  return (
    <div className="agentclient-value">
      <p className="t-label agentclient-value-label">{label}</p>
      <div className="agentclient-value-row">
        <CopyableText text={text} label={label} />
        <Button
          onClick={copy.copy}
          aria-label={t(
            shown
              ? "agentClients.value.copiedNamed"
              : "agentClients.value.copyNamed",
            { value: label },
          )}
        >
          {shown
            ? t("agentClients.value.copied")
            : t("agentClients.value.copy")}
        </Button>
      </div>
      {copy.notice}
    </div>
  );
}

const GEMINI_STEPS: readonly MessageKey[] = [
  "agentClients.gemini.step1",
  "agentClients.gemini.step2",
  "agentClients.gemini.step3",
  "agentClients.gemini.step4",
];

function GeminiSteps() {
  const t = useT();
  return (
    <section className="agentclient-steps">
      <Heading size="small" as="h3">
        {t("agentClients.gemini.title")}
      </Heading>
      <ol>
        {GEMINI_STEPS.map((key) => (
          <li key={key}>{t(key)}</li>
        ))}
      </ol>
    </section>
  );
}
