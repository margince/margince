// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { formatFixture } from "./import.fixtures";
import { ColumnGuide, ColumnGuideButton } from "./import.guide";

// What goes in one column, opened where the column is named. The words come
// from the server's format, the ones the template's README and the agent read.

const meta: Meta = {
  title: "Settings/Data/Data import/Column guide",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const [companies, contacts] = formatFixture.files;
const name = companies?.columns[1];
const tier = companies?.columns[3];
const companyRef = contacts?.columns[2];

function opened(label: string) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: label }),
    );
  };
}

/** A core column a new row needs, opened. */
export const NeededOnANewRow: Story = {
  render: () => (
    <StoryProviders>
      {name ? <ColumnGuide column={name} file="companies.csv" /> : null}
    </StoryProviders>
  ),
  play: opened("display_name"),
};

/** A custom field, named by its label and marked as one. */
export const CustomField: Story = {
  render: () => (
    <StoryProviders>
      {tier ? <ColumnGuide column={tier} file="companies.csv" /> : null}
    </StoryProviders>
  ),
  play: opened("Tier"),
};

/** The `?` beside a field already chosen on the map, in dark. */
export const BesideAChosenFieldDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      {companyRef ? (
        <ColumnGuideButton column={companyRef} file="contacts.csv" />
      ) : null}
    </StoryProviders>
  ),
  play: opened("Guide for company_ref"),
};
