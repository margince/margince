import { useId, useState } from "react";
import type { components } from "../api/schema";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import { downloadBytes } from "./download";

export type AiCallDetail = Pick<
  components["schemas"]["AiCall"],
  "task" | "occurred_at" | "payload"
>;

type CapturedRequest = {
  system?: string;
  messages?: { role: string; content: string }[];
};

// A captured message's keys are read case-insensitively: model.Message
// carries no JSON tags, so payloads captured before that was fixed stored
// "Role"/"Content" while current captures store lowercase — both must
// yield a usable `input`, never a silently empty one.
function stringField(source: object, ...keys: string[]): string | undefined {
  for (const key of keys) {
    if (key in source) {
      const value = (source as Record<string, unknown>)[key];
      if (typeof value === "string") return value;
    }
  }
  return undefined;
}

function capturedRequest(value: unknown): CapturedRequest {
  if (typeof value !== "object" || value === null) return {};
  const system = stringField(value, "system", "System");
  const messages: CapturedRequest["messages"] = [];
  if ("messages" in value && Array.isArray(value.messages)) {
    for (const message of value.messages) {
      if (typeof message !== "object" || message === null) continue;
      const role = stringField(message, "role", "Role");
      const content = stringField(message, "content", "Content");
      if (role !== undefined && content !== undefined) {
        messages.push({ role, content });
      }
    }
  }
  return { system, messages };
}

function blockScalar(text: string): string {
  const body = text
    .split("\n")
    .map((line) => `  ${line}`)
    .join("\n");
  return `|-\n${body}`;
}

function scenarioSlug(name: string): string {
  return (
    name
      .toLowerCase()
      .replaceAll(/[^a-z0-9]+/g, "_")
      // The lookbehind keeps the trailing run from being retried from every
      // underscore in it, which is quadratic on a name that is mostly separators.
      .replaceAll(/^_+|(?<!_)_+$/g, "")
  );
}

export function scenarioYaml(call: AiCallDetail, name: string): string {
  const request = capturedRequest(call.payload?.request);
  const input = (request.messages ?? [])
    .map((message) => `${message.role}: ${message.content}`)
    .join("\n");
  return [
    `name: ${scenarioSlug(name)}`,
    `task: ${call.task}`,
    "source: run_export",
    "sanitized_by: unreviewed",
    `system: ${blockScalar(request.system ?? "")}`,
    `input: ${blockScalar(input)}`,
    "expect:",
    "  structural: []",
    "",
  ].join("\n");
}

export function ExportScenarioDialog({
  call,
  onClose,
}: Readonly<{ call: AiCallDetail; onClose: () => void }>) {
  const t = useT();
  const defaultName = `${call.task}_run_${call.occurred_at.slice(0, 10).replaceAll("-", "")}`;
  const headingId = useId();
  const [name, setName] = useState(defaultName);
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const yaml = scenarioYaml(call, name);
  // A string response is shown verbatim (real newlines) rather than as an
  // escaped JSON string — the reviewer authors `expect:` from this text.
  const response = call.payload?.response;
  const responseText =
    typeof response === "string" ? response : JSON.stringify(response, null, 2);

  async function copyYaml() {
    if (!navigator.clipboard) {
      setCopyFailed(true);
      return;
    }
    try {
      await navigator.clipboard.writeText(yaml);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  }

  function downloadYaml() {
    downloadBytes(yaml, `${scenarioSlug(name)}.yaml`, "application/yaml");
  }

  return (
    <Modal open onClose={onClose} labelledBy={headingId} size="wide">
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("aiexport.title")}
      </h2>
      <div className="form-stack">
        <Field label={t("aiexport.nameLabel")}>
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
        {/* The PII gate before copy or download. It stands as the CONTROL it
            is: a Callout around it was the primitive doing a fieldset's job —
            a notice says something about the surface, and a tick the reader
            must set is not that. The tick is the design system's own, so the
            words are half the click target rather than a label threaded to a
            hand-written id, and they carry the consequence themselves. */}
        <Checkbox
          label={t("aiexport.checklist")}
          checked={acknowledged}
          onChange={(event) => setAcknowledged(event.target.checked)}
        />
        <div className="field">
          <span className="code-label t-eyebrow">
            {t("aiexport.previewLabel")}
          </span>
          <pre className="code-block">{yaml}</pre>
        </div>
        <div className="field">
          <span className="code-label t-eyebrow">
            {t("aiexport.responseLabel")}
          </span>
          <pre className="code-block">{responseText}</pre>
        </div>
        {copyFailed && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("aiexport.copyFailedTitle")}
          >
            {t("aiexport.copyFailed")}
          </Callout>
        )}
      </div>
      <div className="actions">
        <Button onClick={onClose}>{t("aiexport.close")}</Button>
        <Button disabled={!acknowledged} onClick={() => void copyYaml()}>
          {copied ? t("aiexport.copied") : t("aiexport.copy")}
        </Button>
        <Button
          variant="primary"
          disabled={!acknowledged}
          onClick={downloadYaml}
        >
          {t("aiexport.download")}
        </Button>
      </div>
    </Modal>
  );
}
