// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { viewMenuName } from "../testing/listview.names";
import { playOpenView, playOpenViewStep } from "../testing/listview.play";
import { Button, SegmentedControl } from "./atoms";
import { type ListChip, ListSurface } from "./listsurface";
import { ListViewMenu } from "./listviewmenu";

// A list's View button and its panel: a popover from the fold up, a sheet
// below it.

const meta: Meta = {
  title: "Components/Overlays and layering/List view menu",
  component: ListViewMenu,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

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
    options: [
      { value: "u1", label: "Lena Brandt" },
      { value: "u2", label: "Otto Fischer" },
    ],
  },
  {
    key: "country",
    label: "Country",
    allLabel: "Any country",
    options: [{ value: "DE", label: "Germany" }],
  },
];

function Surface({
  initialFilters = {},
  initialSort = "-created_at",
}: Readonly<{
  initialFilters?: Record<string, string>;
  initialSort?: string;
}>) {
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState(initialSort);
  const [chosen, setChosen] = useState(initialFilters);
  const [archived, setArchived] = useState(false);
  const [layout, setLayout] = useState<"table" | "board">("table");
  const [view, setView] = useState(0);
  const changed = Object.values(chosen).some(Boolean);
  return (
    <ListSurface
      title="Deals"
      views={[{ label: "All" }, { label: "Mine" }]}
      activeView={changed ? -1 : view}
      onViewChange={setView}
      search={{ value: search, onChange: setSearch }}
      sort={{ value: sort, onChange: setSort, initial: "-created_at" }}
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
      changed={changed}
      layout={
        <SegmentedControl
          label="Layout"
          options={["table", "board"]}
          value={layout}
          onChange={setLayout}
          labels={{ table: "Table", board: "Board" }}
        />
      }
      viewRows={[
        {
          key: "pipeline",
          label: "Pipeline",
          value: "Sales",
          step: () => <p className="lt-fvalue-status">Sales</p>,
        },
      ]}
      saveView={<Button>Save view</Button>}
    >
      <p>Rows</p>
    </ListSurface>
  );
}

// Two filters apply and the list has left its view. View counts them and
// carries the dot, and the filters stand in one row under the search.
export const AppliedAndChanged: Story = {
  render: () => (
    <Surface initialFilters={{ stage: "qualified", owner: "u1" }} />
  ),
  play: async ({ canvasElement }) => {
    const view = await within(canvasElement).findByRole("button", {
      name: "View, 2 filters, changed",
    });
    await expect(view).toBeVisible();
  },
};

// The search and View stand at one height.
async function expectOneHeight(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const search = await canvas.findByRole("searchbox", { name: "Search" });
  const view = await canvas.findByRole("button", { name: /^View/ });
  await waitFor(() => expect(view).toBeVisible());
  const field = search.closest(".lt-search");
  await expect(field?.getBoundingClientRect().height).toBe(
    view.getBoundingClientRect().height,
  );
}

export const OneHeight: Story = {
  render: () => <Surface />,
  play: async ({ canvasElement }) => expectOneHeight(canvasElement),
};

export const PhoneOneHeight: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <Surface />,
  play: async ({ canvasElement }) => expectOneHeight(canvasElement),
};

// A dial off its default carries a dot, and its name says "changed".
export const ChangedRows: Story = {
  render: () => <Surface initialFilters={{ owner: "u1" }} initialSort="name" />,
  play: async ({ canvasElement }) => {
    const panel = within(await playOpenView(canvasElement));
    await expect(
      panel.getByRole("button", { name: "Filters, Owner, changed" }),
    ).toBeVisible();
    await expect(
      panel.getByRole("button", { name: "Sort, Name, ascending, changed" }),
    ).toBeVisible();
    await expect(
      panel.getByRole("button", { name: "Pipeline, Sales" }),
    ).toBeVisible();
  },
};

export const PhoneSheetChanged: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <Surface initialFilters={{ owner: "u1" }} initialSort="name" />,
  play: async ({ canvasElement }) => {
    const sheet = within(await playOpenView(canvasElement));
    await expect(
      sheet.getByRole("button", { name: "Filters, Owner, changed" }),
    ).toBeVisible();
  },
};

export const PanelOpen: Story = {
  render: () => <Surface initialFilters={{ stage: "qualified" }} />,
  play: async ({ canvasElement }) => {
    const panel = await playOpenView(canvasElement);
    await expect(
      within(panel).getByRole("button", { name: "Sort, Created, descending" }),
    ).toBeVisible();
  },
};

export const ArchivedPressed: Story = {
  render: () => <Surface />,
  play: async ({ canvasElement }) => {
    const panel = await playOpenView(canvasElement);
    const archived = within(panel).getByRole("button", {
      name: "Show archived",
    });
    await userEvent.click(archived);
    await waitFor(() =>
      expect(archived).toHaveAttribute("aria-pressed", "true"),
    );
  },
};

// A row opens its step in place of the rows, headed by the way back.
export const SortStepOpen: Story = {
  render: () => <Surface />,
  play: async ({ canvasElement }) => {
    await playOpenViewStep(canvasElement, "Sort");
  },
};

export const FiltersStepOpen: Story = {
  render: () => <Surface />,
  play: async ({ canvasElement }) => {
    await playOpenViewStep(canvasElement, "Filters");
  },
};

// An applied filter stands in the Filters step with its value; its own step
// opens on that value and ends with Delete filter.
async function openAppliedOwner(canvasElement: HTMLElement) {
  const step = within(await playOpenViewStep(canvasElement, "Filters"));
  const owner = await step.findByRole("button", {
    name: "Owner, Lena Brandt, changed",
  });
  await waitFor(() => expect(owner).toBeVisible());
  await userEvent.click(owner);
  const panel = within(
    await screen.findByRole("dialog", { name: viewMenuName() }),
  );
  const values = within(await panel.findByRole("group", { name: "Owner" }));
  await expect(
    values.getByRole("radio", { name: "Lena Brandt" }),
  ).toBeChecked();
  await expect(
    values.getByRole("button", { name: "Delete filter" }),
  ).toBeVisible();
}

export const AppliedFilterStep: Story = {
  render: () => <Surface initialFilters={{ owner: "u1" }} />,
  play: async ({ canvasElement }) => openAppliedOwner(canvasElement),
};

export const PhoneAppliedFilterStep: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <Surface initialFilters={{ owner: "u1" }} />,
  play: async ({ canvasElement }) => openAppliedOwner(canvasElement),
};

// On a phone the panel is a sheet over the page, headed View.
export const PhoneSheet: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <Surface initialFilters={{ stage: "qualified" }} />,
  play: async ({ canvasElement }) => {
    const sheet = await playOpenView(canvasElement);
    await expect(sheet).toHaveAttribute("aria-modal", "true");
  },
};
