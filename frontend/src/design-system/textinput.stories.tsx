// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Field, TextInput } from "./atoms";

const meta: Meta<typeof TextInput> = {
  title: "Components/Forms and input/Text input",
  component: TextInput,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TextInput>;

// Labelled through `Field`, the way a form gives it a name.
export const States: Story = {
  render: () => (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Field label="Deal name">
        {(control) => <TextInput {...control} placeholder="Globex renewal" />}
      </Field>
      <Field label="Company website">
        {(control) => (
          <TextInput {...control} type="url" defaultValue="https://acme.de" />
        )}
      </Field>
      <Field label="Workspace id">
        {(control) => (
          <TextInput {...control} defaultValue="ws-4f2a" disabled />
        )}
      </Field>
    </div>
  ),
};
