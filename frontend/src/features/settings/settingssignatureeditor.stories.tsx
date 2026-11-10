// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import {
  type SignatureDraft,
  SignatureFields,
} from "./settingssignatureeditor";

function Editor({
  mode,
}: Readonly<{ mode: "plain" | "html" | "template" | "retained" }>) {
  const [draft, setDraft] = useState<SignatureDraft>({
    body: "Mira Voss",
    html: "<p><b>Mira Voss</b></p>",
    title: "Head of Sales",
    phone: "+49 711 555 0100",
    format: "html",
  });
  return (
    <StoryProviders>
      <div className="form-stack">
        <SignatureFields
          mode={mode}
          shown={draft}
          disabled={false}
          change={(field, value) =>
            setDraft((before) => ({ ...before, [field]: value }))
          }
        />
      </div>
    </StoryProviders>
  );
}
const meta = {
  title: "Settings/Across pages/Signature fields",
  component: SignatureFields,
  parameters: { layout: "padded" },
  args: {
    shown: {
      body: "Mira Voss",
      html: "",
      title: "",
      phone: "",
      format: "text",
    },
    mode: "plain",
    disabled: false,
    change: fn(),
  },
} satisfies Meta<typeof SignatureFields>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Plain: Story = { render: () => <Editor mode="plain" /> };
export const Html: Story = { render: () => <Editor mode="html" /> };
export const Template: Story = { render: () => <Editor mode="template" /> };
export const Retained: Story = { render: () => <Editor mode="retained" /> };
