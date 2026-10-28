// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId } from "react";
import { useObjectCustomFields } from "../../components/customfields.form";
import { Button, Modal } from "../../design-system/atoms";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  WEB_FORMS_GUIDE,
  type WebForm,
  type WebFormSender,
} from "./webformkinds";
import {
  browserSnippet,
  intakeUrlOf,
  SECRET_VARIABLE,
  serverSnippet,
} from "./webformsnippet";

// What a step hands the reader to copy, if anything.
type StepValue = "address" | "secret" | "snippet";
type Step = Readonly<{ text: MessageKey; value?: StepValue }>;

const TEST_STEP: Step = { text: "webForms.steps.test" };

// Each sender's setup, in the order the reader clicks through it.
const STEPS: Readonly<Record<WebFormSender, readonly Step[]>> = {
  browser: [
    { text: "webForms.steps.browser.address", value: "address" },
    { text: "webForms.steps.browser.builder" },
    { text: "webForms.steps.browser.fields" },
    { text: "webForms.steps.browser.code", value: "snippet" },
    TEST_STEP,
  ],
  typeform: [
    { text: "webForms.steps.typeform.open" },
    { text: "webForms.steps.endpoint", value: "address" },
    { text: "webForms.steps.typeform.secret", value: "secret" },
    TEST_STEP,
  ],
  tally: [
    { text: "webForms.steps.tally.open" },
    { text: "webForms.steps.endpoint", value: "address" },
    { text: "webForms.steps.tally.secret", value: "secret" },
    TEST_STEP,
  ],
  server: [
    { text: "webForms.steps.server.secret", value: "secret" },
    { text: "webForms.steps.server.address", value: "address" },
    { text: "webForms.steps.server.code", value: "snippet" },
    TEST_STEP,
  ],
  webflow: [
    { text: "webForms.steps.webflow.create", value: "address" },
    { text: "webForms.steps.webflow.secret" },
    TEST_STEP,
  ],
};

function snippetOf(form: WebForm, url: string): string {
  return form.sender === "server"
    ? serverSnippet(form, url)
    : browserSnippet(form, url);
}

type Copy = ReturnType<typeof useClipboardCopy>;

function CopyBlock({
  text,
  label,
  testId,
  copy,
}: Readonly<{ text: string; label: string; testId: string; copy: Copy }>) {
  return (
    <div className="webform-step-value">
      <CopyableText text={text} label={label} testId={testId} />
      <Button data-testid={`${testId}-copy`} onClick={copy.copy}>
        {copy.label}
      </Button>
      {copy.notice}
    </div>
  );
}

// How a website posts to one form, as numbered steps with what each one needs
// to copy. Right after a create or rotate it also shows the minted secret, this
// once; closing destroys that copy.
export function WebFormSetupModal({
  form,
  secret,
  origin = globalThis.location.origin,
  onClose,
}: Readonly<{
  form: WebForm;
  secret?: string;
  origin?: string;
  onClose: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  const url = intakeUrlOf(form, origin);
  const snippet = snippetOf(form, url);
  const leadFields = useObjectCustomFields("lead").fields;
  const copied = t("webForms.setup.copied");
  const remedy = t("webForms.setup.copyFailed");
  const copies: Readonly<Record<StepValue, Copy>> = {
    address: useClipboardCopy(url, {
      copy: t("webForms.setup.copyAddress"),
      copied,
      remedy,
    }),
    secret: useClipboardCopy(secret ?? "", {
      copy: t("webForms.setup.copySecret"),
      copied,
      remedy,
    }),
    snippet: useClipboardCopy(snippet, {
      copy: t("webForms.setup.copy"),
      copied,
      remedy,
    }),
  };
  const handOver = (value: StepValue) => {
    switch (value) {
      case "address":
        return (
          <CopyBlock
            text={url}
            label={t("webForms.setup.address")}
            testId="web-form-address"
            copy={copies.address}
          />
        );
      case "snippet":
        return (
          <CopyBlock
            text={snippet}
            label={t("webForms.setup.snippet")}
            testId="web-form-snippet"
            copy={copies.snippet}
          />
        );
      case "secret":
        return secret ? (
          <CopyBlock
            text={secret}
            label={t("webForms.setup.secret")}
            testId="web-form-secret"
            copy={copies.secret}
          />
        ) : (
          <p className="t-caption">{t("webForms.steps.secretShownOnce")}</p>
        );
    }
  };
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="confirm">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("webForms.setup.title", { name: form.name })}
      </Heading>
      <div className="form-stack">
        <ol className="webform-steps">
          {STEPS[form.sender].map((step) => (
            <li key={step.text}>
              <p>{t(step.text, { name: SECRET_VARIABLE })}</p>
              {step.value && handOver(step.value)}
            </li>
          ))}
        </ol>
        {leadFields.length > 0 && (
          <p data-testid="web-form-custom-fields">
            {t("webForms.setup.customFields", {
              fields: leadFields
                .map((field) => `${field.column_name} (${field.label})`)
                .join(", "),
            })}
          </p>
        )}
        <a
          className="t-caption"
          href={WEB_FORMS_GUIDE}
          target="_blank"
          rel="noreferrer"
        >
          {t("webForms.guide")}
        </a>
        {secret && !copies.secret.copied && (
          // ds:ignore a caution, not a refusal
          <p className="webform-secret-caution">
            {t("webForms.setup.leaveWarning")}
          </p>
        )}
      </div>
      <div className="actions">
        <Button variant="primary" onClick={onClose}>
          {t("webForms.setup.done")}
        </Button>
      </div>
    </Modal>
  );
}
