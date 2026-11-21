// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, TextInput } from "../design-system/atoms";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

/** The address of a linked document. Its hint follows the page being read. */
export function LinkAddressField({
  url,
  onChange,
  hint,
}: Readonly<{
  url: string;
  onChange: (url: string) => void;
  hint: MessageKey | undefined;
}>) {
  const t = useT();
  return (
    <Field
      label={t("docs.add.url")}
      hint={t(hint ?? "docs.add.urlHint")}
      hintLive
    >
      {(control) => (
        <TextInput
          {...control}
          type="url"
          inputMode="url"
          autoComplete="off"
          placeholder={t("docs.add.urlPlaceholder")}
          value={url}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}
