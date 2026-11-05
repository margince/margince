// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { type Locale, LocaleProvider } from "../i18n";
import { DateInput, type ISODate } from "./dateinput";
import { Field } from "./field";

const TODAY = "2026-05-20";

const meta: Meta<typeof DateInput> = {
  title: "Components/Forms and input/Date input",
  component: DateInput,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DateInput>;

function Due(
  props: Readonly<{
    label?: string;
    locale?: Locale;
    initial?: ISODate | "";
    min?: ISODate;
    max?: ISODate;
    disabled?: boolean;
    readOnly?: boolean;
  }>,
) {
  const { label = "Due date", locale = "en", initial = "", ...rest } = props;
  const [day, setDay] = useState<ISODate | "">(initial);
  return (
    <LocaleProvider initial={locale}>
      <Field label={label}>
        {(control) => (
          <DateInput
            {...control}
            {...rest}
            today={TODAY}
            value={day}
            onChange={(event) => setDay(event.target.value)}
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
      <Due />
    </div>
  ),
};

// One day, 6 May, in each UI language's own pattern.
export const FilledInEachLanguage: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Due label="Due date (en)" initial="2026-05-06" />
      <Due label="Fällig am (de)" locale="de" initial="2026-05-06" />
      <Due label="Ngày đến hạn (vi)" locale="vi" initial="2026-05-06" />
    </div>
  ),
};

export const InvalidTyped: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Due />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const box = await within(canvasElement).findByRole("textbox", {
      name: "Due date",
    });
    await userEvent.type(box, "31/02/2026");
    await userEvent.tab();
    await expect(box.getAttribute("aria-invalid")).toBe("true");
    await expect(
      await within(canvasElement).findByRole("alert"),
    ).toHaveTextContent("Enter a date as DD/MM/YYYY.");
  },
};

export const DisabledAndReadOnly: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Due label="Disabled" initial="2026-05-06" disabled />
      <Due label="Read-only" initial="2026-05-06" readOnly />
    </div>
  ),
};

// Days before the 10th and after the 30th are struck through and refused.
export const MinMaxRefusal: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Due initial="2026-05-12" min="2026-05-10" max="2026-05-30" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const opener = await within(canvasElement).findByRole("button", {
      name: "Choose date: Due date",
    });
    await userEvent.click(opener);
    await expect(
      await screen.findByRole("dialog", { name: "Choose date: Due date" }),
    ).toBeTruthy();
  },
};

export const OpenInVietnamese: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Due label="Ngày đến hạn" locale="vi" initial="2026-05-06" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const opener = await within(canvasElement).findByRole("button", {
      name: "Chọn ngày: Ngày đến hạn",
    });
    await userEvent.click(opener);
    await expect(
      await screen.findByRole("dialog", { name: "Chọn ngày: Ngày đến hạn" }),
    ).toBeTruthy();
  },
};

export const OpenInGerman: Story = {
  render: () => (
    <div className="form-stack" style={column}>
      <Due label="Fällig am" locale="de" initial="2026-05-06" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const opener = await within(canvasElement).findByRole("button", {
      name: "Datum wählen: Fällig am",
    });
    await userEvent.click(opener);
    await expect(
      await screen.findByRole("dialog", { name: "Datum wählen: Fällig am" }),
    ).toBeTruthy();
  },
};

export const AtPhoneWidth: Story = {
  render: () => (
    <div className="form-stack">
      <Due initial="2026-05-06" />
    </div>
  ),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const opener = await within(canvasElement).findByRole("button", {
      name: "Choose date: Due date",
    });
    await userEvent.click(opener);
    await expect(
      await screen.findByRole("dialog", { name: "Choose date: Due date" }),
    ).toBeTruthy();
  },
};

// The field sits in the right-hand column of a phone form, as in Log activity.
// The calendar hangs off the field's end and stays inside the page gutter.
export const AtPhoneEndEdge: Story = {
  render: () => (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "1fr 1fr",
        gap: "var(--space-3)",
      }}
    >
      <Due label="Starts" initial="2026-05-04" />
      <Due label="Due date" initial="2026-05-06" />
    </div>
  ),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Choose date: Due date",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Choose date: Due date",
    });
    await waitFor(() => {
      const box = dialog.getBoundingClientRect();
      expect(box.left).toBeGreaterThanOrEqual(16);
      expect(box.right).toBeLessThanOrEqual(globalThis.innerWidth - 16);
    });
  },
};
