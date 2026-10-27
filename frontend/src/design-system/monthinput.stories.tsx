// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { type Locale, LocaleProvider } from "../i18n";
import { Field } from "./field";
import { MonthInput, type YearMonth } from "./monthinput";

const meta: Meta<typeof MonthInput> = {
  title: "Components/Forms and input/Month input",
  component: MonthInput,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof MonthInput>;

function Period({
  locale = "en",
  initial = "",
}: Readonly<{ locale?: Locale; initial?: YearMonth | "" }>) {
  const [month, setMonth] = useState<YearMonth | "">(initial);
  return (
    <LocaleProvider initial={locale}>
      <Field label="Month starting">
        {(control) => (
          <MonthInput {...control} value={month} onChange={setMonth} />
        )}
      </Field>
    </LocaleProvider>
  );
}

export const AnyMonth: Story = { render: () => <Period /> };

/** The month by name in the UI language. */
export const German: Story = {
  render: () => <Period locale="de" initial="2026-03" />,
};
