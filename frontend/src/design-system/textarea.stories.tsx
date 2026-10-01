// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Field, Textarea } from "./atoms";

const meta: Meta<typeof Textarea> = {
  title: "Components/Forms and input/Textarea",
  component: Textarea,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Textarea>;

export const States: Story = {
  render: () => (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Field label="Note">
        {(control) => (
          <Textarea {...control} rows={3} placeholder="What was agreed?" />
        )}
      </Field>
      <Field label="Call summary" hint="Only the deal's followers see this.">
        {(control) => (
          <Textarea
            {...control}
            rows={4}
            defaultValue={
              "Renewal terms agreed on the call: 36 months, payment 30 days net.\nLegal review pending."
            }
          />
        )}
      </Field>
      <Field label="Imported note">
        {(control) => (
          <Textarea
            {...control}
            rows={2}
            defaultValue="Captured from the old CRM."
            disabled
          />
        )}
      </Field>
    </div>
  ),
};
