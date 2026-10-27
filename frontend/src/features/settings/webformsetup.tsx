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
import { isBuilder, type WebForm } from "./webformkinds";
import {
  browserSnippet,
  intakeUrlOf,
  SECRET_VARIABLE,
  serverSnippet,
} from "./webformsnippet";

function snippetOf(form: WebForm, url: string): string | null {
  switch (form.sender) {
    case "browser":
      return browserSnippet(form, url);
    case "server":
      return serverSnippet(form, url);
    default:
      return null;
  }
}

// How a website posts to one form: its address and the code to paste. Right
// after a create or rotate, it also shows the server secret, this once. The copy button
// takes the secret while there is one, since closing destroys it.
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
  const copy = useClipboardCopy(secret ?? snippet ?? url, {
    copy: secret ? t("webForms.setup.copySecret") : t("webForms.setup.copy"),
    copied: t("webForms.setup.copied"),
    remedy: t("webForms.setup.copyFailed"),
  });
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="confirm">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("webForms.setup.title", { name: form.name })}
      </Heading>
      <div className="form-stack">
        <p>{t(`webForms.setup.how.${form.sender}`)}</p>
        <CopyableText
          text={url}
          label={t("webForms.setup.address")}
          testId="web-form-address"
        />
        {secret && (
          <>
            <p>
              {t("webForms.setup.secretWarning", { name: SECRET_VARIABLE })}
            </p>
            <CopyableText
              text={secret}
              label={t("webForms.setup.secret")}
              testId="web-form-secret"
            />
          </>
        )}
        {snippet && (
          <CopyableText
            text={snippet}
            label={t("webForms.setup.snippet")}
            testId="web-form-snippet"
          />
        )}
        {isBuilder(form.sender) && <p>{t("webForms.setup.builderSecret")}</p>}
        {leadFields.length > 0 && (
          <p data-testid="web-form-custom-fields">
            {t("webForms.setup.customFields", {
              fields: leadFields
                .map((field) => `${field.column_name} (${field.label})`)
                .join(", "),
            })}
          </p>
        )}
        {copy.notice}
        {secret && !copy.copied && (
          // ds:ignore a caution, not a refusal
          <p className="webform-secret-caution">
            {t("webForms.setup.leaveWarning")}
          </p>
        )}
      </div>
      <div className="actions">
        <Button onClick={onClose}>{t("webForms.setup.done")}</Button>
        <Button variant="primary" onClick={copy.copy}>
          {copy.label}
        </Button>
      </div>
    </Modal>
  );
}
