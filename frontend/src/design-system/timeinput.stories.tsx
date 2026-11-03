// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { type Locale, LocaleProvider } from "../i18n";
import { Field } from "./field";
import { type ClockTime, TimeInput } from "./timeinput";

const meta: Meta<typeof TimeInput> = {
  title: "Components/Forms and input/Time input",
  component: TimeInput,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TimeInput>;

function Starts({
  locale = "en",
  initial = "",
  untilMidnight,
}: Readonly<{
  locale?: Locale;
  initial?: ClockTime | "";
  untilMidnight?: boolean;
}>) {
  const [time, setTime] = useState<ClockTime | "">(initial);
  return (
    <LocaleProvider initial={locale}>
      <Field label="Starts">
        {(control) => (
          <TimeInput
            {...control}
            untilMidnight={untilMidnight}
            value={time}
            onChange={(event) => setTime(event.target.value)}
          />
        )}
      </Field>
    </LocaleProvider>
  );
}

const column = { maxWidth: "22rem" };

export const Empty: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Starts />
    </div>
  ),
};

/** The UI language's own clock, whatever language the browser speaks. */
export const Vietnamese: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Starts locale="vi" initial="21:05" />
    </div>
  ),
};

/** The end of a working day that runs until midnight. */
export const UntilMidnight: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Starts initial="24:00" untilMidnight />
    </div>
  ),
};
