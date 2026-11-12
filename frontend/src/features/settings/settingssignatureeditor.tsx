// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { SignatureHtml } from "../../components/signaturehtml";
import { Field, Textarea, TextInput } from "../../design-system/atoms";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

// The server's cap on personal HTML, counted on the trimmed raw input.
// backend/gates/frontendsignaturecap_test.go fails when it and the contract disagree.
export const SIGNATURE_HTML_MAX_CHARS = 10_000;
// The counter shows once the input passes this share of the cap.
const COUNTER_FROM = 0.8;

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
  htmlHint?:
    | "settings.signatureHtmlHintTemplate"
    | "settings.signatureHtmlHintPlain";
  disabled: boolean;
  change: (
    field: Exclude<keyof SignatureDraft, "format">,
    value: string,
  ) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const max = formatNumber(SIGNATURE_HTML_MAX_CHARS, locale);
  // Code points, because the server counts runes: "𝐀" is one, not two units.
  const count = [...shown.html.trim()].length;
  const counter = count > SIGNATURE_HTML_MAX_CHARS * COUNTER_FROM && (
    <span
      className={count > SIGNATURE_HTML_MAX_CHARS ? "field-error" : undefined}
    >
      {plural("settings.signatureHtmlCount", count, {
        count: formatNumber(count, locale),
        max,
      })}
    </span>
  );
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
          {(mode === "html" || counter) && (
            <p className="t-caption">
              {mode === "html" &&
                t(htmlHint ?? "settings.signatureHtmlHintPlain", { max })}{" "}
              {counter}
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
