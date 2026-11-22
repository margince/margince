// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, TextInput } from "../../design-system/atoms";
import { useT } from "../../i18n";

// Where Azure processes an Azure OpenAI call. The server maps each region to a
// country and blocks one it does not know. The field says so up front.

export const AZURE_PROVIDER = "azure_openai";

export function AzureRegionField({
  value,
  disabled,
  onChange,
}: Readonly<{
  value: string;
  disabled: boolean;
  onChange: (region: string) => void;
}>) {
  const t = useT();
  return (
    <Field
      label={t("aiRouting.azureRegion.label")}
      hint={t("aiRouting.azureRegion.help")}
    >
      {(control) => (
        <TextInput
          {...control}
          value={value}
          disabled={disabled}
          placeholder="swedencentral"
          onChange={(e) => onChange(e.target.value.trim().toLowerCase())}
        />
      )}
    </Field>
  );
}
