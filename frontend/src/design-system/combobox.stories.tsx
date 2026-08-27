// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Field } from "./atoms";
import { ComboBox, type ComboBoxSuggestion } from "./combobox";

/**
 * The combo box: a text field that also offers, where the list is help rather
 * than a constraint.
 *
 * What these stories are for, in order of what they actually catch: it has to
 * sit on the same baseline as a `TextInput` and a `Select` beside it (Fields),
 * a typed value nobody suggested has to survive (Off The List), and with
 * nothing to suggest it has to be an ordinary text box with no chevron
 * promising a list that is not there (Nothing To Suggest). Flip the Theme
 * toolbar to see the dark rendering — every value here is a token.
 */
const meta = {
  title: "Design System/ComboBox",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

// The real thing this was built for: model ids off an installation's price
// sheet, which are long, namespaced, and nothing anyone types from memory.
const MODELS: readonly ComboBoxSuggestion[] = [
  { value: "gemini-3.5-flash", hint: "premium" },
  { value: "gemini-3.1-flash-lite", hint: "cheap" },
  { value: "gemini-3.1-pro-preview", hint: "frontier" },
  { value: "mistralai/mistral-small-3.2-24b-instruct" },
  { value: "mistralai/mistral-large-2512" },
  { value: "deepseek/deepseek-v4-flash" },
  { value: "openai/gpt-oss-120b" },
];

function Demo({
  label,
  suggestions,
  initial = "",
  hint,
}: Readonly<{
  label: string;
  suggestions: readonly ComboBoxSuggestion[];
  initial?: string;
  hint?: string;
}>) {
  const [value, setValue] = useState(initial);
  return (
    <Field label={label} hint={hint}>
      {(control) => (
        <ComboBox
          {...control}
          value={value}
          onChange={setValue}
          suggestions={suggestions}
          placeholder="model id"
        />
      )}
    </Field>
  );
}

export const Default: Story = {
  render: () => (
    <div style={{ maxWidth: 420 }}>
      <Demo
        label="Model"
        suggestions={MODELS}
        hint="Type any id your vendor offers."
      />
    </div>
  ),
};

/** A value the list does not carry — an operator who knows their own vendor. */
export const OffTheList: Story = {
  render: () => (
    <div style={{ maxWidth: 420 }}>
      <Demo
        label="Model"
        suggestions={MODELS}
        initial="gemini-4-experimental-0731"
      />
    </div>
  ),
};

/** An installation whose price sheet knows nothing about the chosen provider. */
export const NothingToSuggest: Story = {
  render: () => (
    <div style={{ maxWidth: 420 }}>
      <Demo label="Model" suggestions={[]} initial="llama-3.3-70b" />
    </div>
  ),
};

export const Disabled: Story = {
  render: () => (
    <div style={{ maxWidth: 420 }}>
      <Field label="Model">
        {(control) => (
          <ComboBox
            {...control}
            value="gemini-3.1-flash-lite"
            onChange={() => undefined}
            suggestions={MODELS}
            disabled
          />
        )}
      </Field>
    </div>
  ),
};
