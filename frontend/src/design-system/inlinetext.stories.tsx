// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { FieldGrid, FieldRow } from "./fieldgrid";
import { InlineText } from "./inlinetext";

const meta: Meta = {
  title: "Components/Forms and input/Inline text editing",
  component: InlineText,
  decorators: [
    (Story) => (
      <LocaleProvider>
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;
function Example({ multiline = false }: { multiline?: boolean }) {
  const [value, setValue] = useState(
    multiline ? "A paragraph with intentional whitespace." : "14.60",
  );
  return (
    <InlineText
      label={multiline ? "Brief" : "Value"}
      placeholder="Not set"
      value={value}
      multiline={multiline}
      type={multiline ? "text" : "number"}
      step="any"
      canEdit
      onSave={async (next) => setValue(next)}
    />
  );
}
export const FractionalNumber: Story = { render: () => <Example /> };

export const FormattedReading: Story = {
  render: () => (
    <InlineText
      label="Budget"
      placeholder="Not set"
      value="48000"
      display="€48,000.00"
      type="number"
      canEdit
      onSave={async () => undefined}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Change Budget €48,000.00" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Budget" });
    await expect(within(dialog).getByRole("spinbutton")).toHaveValue(48000);
  },
};
export const Paragraph: Story = { render: () => <Example multiline /> };

// A refused save keeps the dialog open on the draft and says why as the
// field's error beneath it.
export const RefusedSave: Story = {
  render: () => (
    <InlineText
      label="Website"
      placeholder="Not set"
      value="acme.example"
      canEdit
      onSave={async () => {
        throw new ProblemError({
          code: "version_skew",
          detail: "Somebody else changed this record.",
        });
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button"));
    const dialog = await screen.findByRole("dialog", { name: "Website" });
    await userEvent.type(within(dialog).getByRole("textbox"), "s{Enter}");
    const refusal = await within(dialog).findByRole("alert");
    await expect(refusal).toHaveTextContent(en["common.versionSkew"]);
    await expect(refusal).toHaveClass("field-error");
  },
};

function DayExample() {
  const [value, setValue] = useState("2026-03-01");
  return (
    <InlineText
      label="Expected close"
      placeholder="Not set"
      value={value}
      type="date"
      canEdit
      onSave={async (next) => setValue(next)}
    />
  );
}

// A day picked in the calendar is the draft; Save stores it.
export const Day: Story = {
  render: () => <DayExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Change Expected close/ }),
    );
    const page = within(canvasElement.ownerDocument.body);
    const dialog = await page.findByRole("dialog", { name: "Expected close" });
    await userEvent.click(
      within(dialog).getByRole("button", { name: /^Choose date/ }),
    );
    await userEvent.click(
      await page.findByRole("button", { name: "Tuesday, 10 March 2026" }),
    );
    await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await expect(
      await canvas.findByRole("button", {
        name: "Change Expected close 2026-03-10",
      }),
    ).toBeVisible();
  },
};

// In a grid: a plain value, a link, an empty field, a suggested value
// and a long run that wraps from the start edge.
function GridExample({ canEdit }: Readonly<{ canEdit: boolean }>) {
  const save = async () => undefined;
  return (
    <FieldGrid>
      <FieldRow label="Legal name">
        <InlineText
          label="Legal name"
          value="Acme Robotics GmbH"
          placeholder="Add legal name"
          canEdit={canEdit}
          onSave={save}
        />
      </FieldRow>
      <FieldRow label="Email">
        <InlineText
          label="Email"
          value="ana@example.com"
          type="email"
          link={
            <a className="link-button" href="mailto:ana@example.com">
              ana@example.com
            </a>
          }
          placeholder="Add email"
          canEdit={canEdit}
          onSave={save}
        />
      </FieldRow>
      <FieldRow label="Website">
        <InlineText
          label="Website"
          value=""
          placeholder="Add website"
          canEdit={canEdit}
          onSave={save}
        />
      </FieldRow>
      <FieldRow label="Role">
        <InlineText
          label="Role"
          value=""
          suggested="Head of Procurement"
          placeholder="Add role"
          canEdit={canEdit}
          onSave={save}
        />
      </FieldRow>
      <FieldRow label="Domain">
        <InlineText
          label="Domain"
          value="industrial-automation-and-robotics-integration.example"
          placeholder="Add domain"
          canEdit={canEdit}
          onSave={save}
        />
      </FieldRow>
    </FieldGrid>
  );
}

export const InGrid: Story = {
  render: () => <GridExample canEdit />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("link", { name: "ana@example.com" }),
    ).toHaveAttribute("href", "mailto:ana@example.com");
    await userEvent.click(canvas.getByRole("button", { name: "Change Email" }));
    const dialog = await screen.findByRole("dialog", { name: "Email" });
    // The dialog fades in from transparent, so visible is a state it reaches.
    await waitFor(() => expect(dialog).toBeVisible());
  },
};

export const InGridDark: Story = {
  render: () => <GridExample canEdit />,
  globals: { theme: "dark" },
};

// A reader who may not edit sees plain values: no fill, no bubble, no pointer.
export const InGridReadOnly: Story = {
  render: () => <GridExample canEdit={false} />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole("button")).toBeNull();
  },
};
