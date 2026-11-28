import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { EmploymentEdit } from "./employmentedit";

const meta: Meta<typeof EmploymentEdit> = {
  title: "Records/Contact 360/Employment correction",
  component: EmploymentEdit,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    contactId: "sample",
    employment: {
      relationship_id: "role",
      company_id: "company",
      is_current_primary: false,
      role: "Advisor",
      employment_status: "unknown",
      started_at: "2020-02-01",
      started_precision: "month",
      version: 1,
    },
    onClose: () => {},
    onSaved: async () => {},
  },
};
export default meta;
type Story = StoryObj<typeof meta>;
export const UnknownDates: Story = {
  play: async () => {
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Save" }));
    const refusal = await screen.findByText("Employment status is required.");
    await waitFor(() => expect(refusal).toBeVisible());
    await user.click(
      screen.getByRole("combobox", {
        name: englishCopy["employment.statusLabel"],
      }),
    );
    expect(
      screen.queryByRole("option", {
        name: englishCopy["employment.status.unknown"],
      }),
    ).toBeNull();
    expect(screen.getAllByRole("option")).toHaveLength(2);
  },
};
export const UnknownDatesDark: Story = {
  ...UnknownDates,
  globals: { theme: "dark" },
};
export const Former: Story = {
  args: {
    employment: {
      relationship_id: "role",
      company_id: "company",
      is_current_primary: false,
      role: "Engineer",
      employment_status: "former",
      started_at: "2020-02-01",
      started_precision: "month",
      ended_at: "2024-05-17",
      ended_precision: "day",
      version: 1,
    },
  },
};
