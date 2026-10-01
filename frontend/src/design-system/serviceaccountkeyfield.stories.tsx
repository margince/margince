// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { ServiceAccountKeyField } from "./serviceaccountkeyfield";

// The key-file control Settings and onboarding share. What a reviewer checks
// here: the box starts empty, the picker sits under it, and a refusal reads as
// the field's own error rather than as a fault in the page.
function Harness({
  initial = "",
  error,
  disabled = false,
}: Readonly<{ initial?: string; error?: string; disabled?: boolean }>) {
  const [value, setValue] = useState(initial);
  return (
    <LocaleProvider initial="en">
      <div style={{ maxWidth: "560px" }}>
        <ServiceAccountKeyField
          value={value}
          onChange={setValue}
          disabled={disabled}
          hint="Sealed in the key vault, never shown again."
          error={error}
        />
      </div>
    </LocaleProvider>
  );
}

const meta: Meta<typeof ServiceAccountKeyField> = {
  title: "Components/Forms and input/Service-account key field",
  component: ServiceAccountKeyField,
};
export default meta;
type Story = StoryObj<typeof ServiceAccountKeyField>;

export const Empty: Story = { render: () => <Harness /> };

export const NotJson: Story = {
  render: () => (
    <Harness
      initial="AIzaSy-an-api-key"
      error="This is not JSON. Paste the whole key file exactly as Google Cloud downloaded it."
    />
  ),
};

// A seat that may not change the credential: the box is disabled and the
// picker is not offered at all.
export const ReadOnly: Story = { render: () => <Harness disabled /> };

export const EmptyDark: Story = {
  globals: { theme: "dark" },
  render: () => <Harness />,
};
