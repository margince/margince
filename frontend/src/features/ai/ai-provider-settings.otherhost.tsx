// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, TextInput } from "../../design-system/atoms";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

// The guide a reader setting up a service by hand is pointed at.
const HOST_GUIDE =
  "https://github.com/margince/margince/blob/main/docs/how-to/connect-a-cloud-model-provider.md";

// What the Other service asks for on each provider. A chat host gets /v1
// appended, Gemini's carries its version, a decision endpoint is used as written.
export type OtherHost = Readonly<{ help: MessageKey; placeholder: MessageKey }>;

/** The host an admin types when no listed service is theirs. */
export function OtherHostFields({
  other,
  typed,
  disabled,
  onChange,
}: Readonly<{
  other: OtherHost;
  typed: string;
  disabled: boolean;
  onChange: (host: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("aiRouting.baseUrl.label")} hint={t(other.help)}>
        {(control) => (
          <TextInput
            {...control}
            value={typed}
            disabled={disabled}
            placeholder={t(other.placeholder)}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
      </Field>
      <a
        className="t-caption"
        href={HOST_GUIDE}
        target="_blank"
        rel="noreferrer"
      >
        {t("aiProviderSettings.host.guide")}
      </a>
    </>
  );
}
