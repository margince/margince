// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { playOpenViewStep } from "../testing/listview.play";
import type { ListChip } from "./listsurface.dials";
import { Toolbar } from "./listtoolbar";

// A list's toolbar: the search, one View button, and the applied filters in a
// row under them.

const CHIPS: readonly ListChip[] = [
  {
    key: "stage",
    label: "Stage",
    allLabel: "Any stage",
    options: [
      { value: "qualified", label: "Qualified" },
      { value: "proposal", label: "Proposal sent" },
    ],
  },
  {
    key: "owner",
    label: "Owner",
    allLabel: "Any owner",
    options: [{ value: "u1", label: "Lena Brandt" }],
  },
];

function ToolbarHost({
  initialFilters,
}: Readonly<{ initialFilters: Record<string, string> }>) {
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState("-created_at");
  const [chosen, setChosen] = useState(initialFilters);
  const [archived, setArchived] = useState(false);
  const [openMenu, setOpenMenu] = useState<string | null>(null);
  return (
    <Toolbar
      search={{ value: search, onChange: setSearch }}
      sort={{ value: sort, onChange: setSort }}
      sortOptions={[
        { field: "name", label: "Name" },
        { field: "created_at", label: "Created" },
      ]}
      chips={CHIPS}
      chosen={chosen}
      onChipChange={(key, value) =>
        setChosen((prev) => ({ ...prev, [key]: value }))
      }
      archived={{ checked: archived, onChange: setArchived }}
      viewRows={[]}
      displayRows={[]}
      displayToggles={[]}
      changed={Object.values(chosen).some(Boolean)}
      openMenu={openMenu}
      setOpenMenu={setOpenMenu}
    />
  );
}

const meta = {
  title: "Components/Text and data display/List toolbar",
  component: ToolbarHost,
  parameters: { layout: "padded" },
  args: { initialFilters: {} },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof ToolbarHost>;
export default meta;

type Story = StoryObj<typeof meta>;

export const SearchAndView: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("combobox", { name: "Search" }),
    ).toBeVisible();
    await expect(canvas.getByRole("button", { name: "View" })).toBeVisible();
  },
};

// View counts the applied filters, and each stands as its own row.
export const FiltersApplied: Story = {
  args: { initialFilters: { stage: "qualified", owner: "u1" } },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("button", { name: "View, 2 filters, changed" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("group", { name: "Stage: Qualified" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("group", { name: "Owner: Lena Brandt" }),
    ).toBeVisible();
  },
};

// A value picked in the Filters step stands at once as its row under the search.
export const FiltersStep: Story = {
  play: async ({ canvasElement }) => {
    const step = within(await playOpenViewStep(canvasElement, "Filters"));
    await userEvent.click(step.getByRole("button", { name: "Stage" }));
    await userEvent.click(
      await screen.findByRole("radio", { name: "Qualified" }),
    );
    await waitFor(() =>
      expect(
        within(canvasElement).getByRole("group", { name: "Stage: Qualified" }),
      ).toBeVisible(),
    );
  },
};

export const FiltersStepDark: Story = {
  ...FiltersStep,
  globals: { theme: "dark" },
};

export const FiltersPhone: Story = {
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
  args: { initialFilters: { stage: "qualified", owner: "u1" } },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("group", {
        name: "Stage: Qualified",
      }),
    ).toBeVisible();
  },
};
