// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { SignatureHtml } from "../../components/signaturehtml";
import { Field, Textarea, TextInput } from "../../design-system/atoms";
import { useT } from "../../i18n";

export type SignatureFormat = "text" | "html";
export type SignatureDraft = Pick<
  components["schemas"]["EmailSignature"],
  "body" | "html" | "title" | "phone"
> & { format: SignatureFormat };

export function SignatureFields({
  shown,
  mode,
  previewHtml,
  htmlHint,
  change,
  disabled,
}: Readonly<{
  shown: SignatureDraft;
  mode: "plain" | "html" | "template" | "retained";
  previewHtml?: string;
  htmlHint?: string;
  disabled: boolean;
  change: (
    field: Exclude<keyof SignatureDraft, "format">,
    value: string,
  ) => void;
}>) {
  const t = useT();
  return (
    <>
      {(mode === "plain" || mode === "retained") && (
        <>
          <Field label={t("settings.signatureLabel")}>
            {(control) => (
              <Textarea
                {...control}
                disabled={disabled}
                rows={5}
                value={shown.body}
                placeholder={t("settings.signaturePlaceholder")}
                onChange={(event) => change("body", event.target.value)}
              />
            )}
          </Field>
          {mode === "plain" && (
            <p className="t-caption">{t("settings.signatureHint")}</p>
          )}
        </>
      )}
      {(mode === "html" || mode === "retained") && (
        <>
          <Field label={t("settings.signatureHtmlLabel")}>
            {(control) => (
              <Textarea
                {...control}
                disabled={disabled}
                rows={8}
                value={shown.html}
                onChange={(event) => change("html", event.target.value)}
              />
            )}
          </Field>
          {mode === "html" && (
            <p className="t-caption">
              {htmlHint ?? t("settings.signatureHtmlHintPlain")}
            </p>
          )}
        </>
      )}
      {(mode === "template" || mode === "retained") && (
        <>
          {mode === "template" && (
            <p className="t-caption">{t("settings.signatureTemplateHint")}</p>
          )}
          <Field label={t("settings.signatureTitle")}>
            {(control) => (
              <TextInput
                {...control}
                disabled={disabled}
                value={shown.title}
                onChange={(event) => change("title", event.target.value)}
              />
            )}
          </Field>
          <Field label={t("settings.signaturePhone")}>
            {(control) => (
              <TextInput
                {...control}
                disabled={disabled}
                value={shown.phone}
                onChange={(event) => change("phone", event.target.value)}
              />
            )}
          </Field>
        </>
      )}
      {previewHtml && (
        <SignatureHtml
          html={previewHtml}
          title={t("settings.signaturePreview")}
        />
      )}
    </>
  );
}
