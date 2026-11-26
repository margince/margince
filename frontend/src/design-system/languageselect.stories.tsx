// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, within } from "storybook/test";
import { LOCALES, type Locale } from "../i18n/locale";
import { Field } from "./atoms";
import { LanguageSelect } from "./languageselect";

const meta: Meta<typeof LanguageSelect> = {
  title: "Components/Forms and input/Language select",
  component: LanguageSelect,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof meta>;

function Picker({
  open = false,
  initial = "de",
  disabled = false,
}: Readonly<{ open?: boolean; initial?: Locale; disabled?: boolean }>) {
  const [language, setLanguage] = useState<Locale>(initial);
  return (
    <Field label="Language">
      {(control) => (
        <LanguageSelect
          {...control}
          languages={LOCALES}
          value={language}
          onChange={setLanguage}
          openOnMount={open}
          disabled={disabled}
        />
      )}
    </Field>
  );
}

export const Closed: Story = { render: () => <Picker /> };

/** Every name in its own language, each option marked with that language. */
export const Open: Story = {
  render: () => <Picker open />,
  play: async () => {
    const option = await screen.findByRole("option", { name: "Tiếng Việt" });
    const label = within(option).getByText("Tiếng Việt");
    await expect(label.getAttribute("lang")).toBe("vi");
  },
};

export const Dark: Story = { ...Open, globals: { theme: "dark" } };

/** Held while the write the pick started is out. */
export const Disabled: Story = { render: () => <Picker disabled /> };
