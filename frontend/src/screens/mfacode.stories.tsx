// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { KeyRound } from "lucide-react";
import { useState } from "react";
import { useT } from "../i18n";
import { MfaCodeField } from "./mfacode";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof MfaCodeField> = {
  title: "Signed out/Second-factor code field",
  component: MfaCodeField,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MfaCodeField>;

function Typed({ refused }: Readonly<{ refused?: boolean }>) {
  const t = useT();
  const [code, setCode] = useState(refused ? "" : "123 456");
  return (
    <MfaCodeField
      label={t("auth.mfaCode")}
      icon={<KeyRound aria-hidden />}
      error={refused ? t("mfa.codeRefused") : undefined}
      value={code}
      onChange={setCode}
    />
  );
}

/** An authenticator code as typed, in its groups. */
export const Filled: Story = {
  render: () => (
    <StoryProviders>
      <Typed />
    </StoryProviders>
  ),
};

/** A refused code: the field is cleared and says why. */
export const Refused: Story = {
  render: () => (
    <StoryProviders>
      <Typed refused />
    </StoryProviders>
  ),
};
