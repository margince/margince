// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Lock, Mail } from "lucide-react";
import { useState } from "react";
import { Field, Textarea, TextInput } from "./atoms";
import { usePasswordReveal } from "./passwordreveal";
import { Select } from "./select";

const meta: Meta<typeof Field> = {
  title: "Components/Forms and input/Field",
  component: Field,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Field>;

function FormColumn() {
  const [stage, setStage] = useState("proposal");
  return (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Field label="Deal name">
        {(control) => <TextInput {...control} defaultValue="Globex renewal" />}
      </Field>
      <Field label="Stage" required>
        {(control) => (
          <Select
            {...control}
            options={[
              { value: "qualify", label: "Qualify" },
              { value: "proposal", label: "Proposal" },
              { value: "won", label: "Won" },
            ]}
            value={stage}
            onChange={setStage}
          />
        )}
      </Field>
      <Field label="Note" hint="Only the deal's followers will see this.">
        {(control) => (
          <Textarea
            {...control}
            rows={3}
            defaultValue="Renewal terms agreed on the call."
          />
        )}
      </Field>
    </div>
  );
}

// Stacked as a form stacks them: the only way to see that type size, padding,
// height and focus ring agree across the three controls.
export const InAForm: Story = {
  render: () => <FormColumn />,
};

function AffordancesColumn() {
  const reveal = usePasswordReveal({
    show: "Show password",
    hide: "Hide password",
  });
  const revealShort = usePasswordReveal({
    show: "Show password",
    hide: "Hide password",
  });
  return (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Field label="Work address" icon={<Mail aria-hidden />}>
        {(control) => (
          <TextInput
            {...control}
            type="email"
            defaultValue="ops@example.com"
            autoComplete="username"
          />
        )}
      </Field>
      <Field
        label="Password"
        icon={<Lock aria-hidden />}
        labelEnd={
          <button type="button" className="link-button">
            Forgot?
          </button>
        }
        hint="At least 12 characters"
        trailing={reveal.trailing}
      >
        {(control) => (
          <TextInput
            {...control}
            type={reveal.type}
            defaultValue="correct horse battery"
            autoComplete="current-password"
          />
        )}
      </Field>
      <Field
        label="New password"
        required
        error="Password is too short. Use at least 12 characters."
        trailing={revealShort.trailing}
      >
        {(control) => (
          <TextInput
            {...control}
            type={revealShort.type}
            defaultValue="short"
            autoComplete="new-password"
          />
        )}
      </Field>
      <Field
        label="Confirm"
        required
        error="Passwords do not match."
        hint="Both fields have to say the same thing."
      >
        {(control) => (
          <TextInput
            {...control}
            type="password"
            defaultValue="something else"
            autoComplete="new-password"
          />
        )}
      </Field>
    </div>
  );
}

// The glyph and the reveal sit inside one outline; `error` is its own slot
// because a refusal and the rule it broke must not share one grey.
export const AffordancesAndRefusal: Story = {
  render: () => <AffordancesColumn />,
};
