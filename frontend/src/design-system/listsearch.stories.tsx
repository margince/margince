// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { foldForMatch } from "../format/collate";
import { LocaleProvider } from "../i18n";
import { ListSearch } from "./listsearch";
import { type ListChip, ListSurface } from "./listsurface";

const company: ListChip = {
  key: "company_id",
  label: "Company",
  allLabel: "Any company",
  options: [],
  search: async (query) =>
    [
      { value: "acme", label: "Acme Engineering" },
      { value: "other", label: "Other company" },
    ].filter((row) => foldForMatch(row.label).includes(foldForMatch(query))),
  appliedName: undefined,
};
const owner: ListChip = {
  key: "owner_id",
  label: "Owner",
  allLabel: "Any owner",
  options: [{ value: "ada", label: "Ada" }],
};
const stage: ListChip = {
  key: "stage_id",
  label: "Stage",
  allLabel: "Any stage",
  options: [{ value: "proposal", label: "Proposal" }],
};
const status: ListChip = {
  key: "status",
  label: "Status",
  allLabel: "Any status",
  options: [{ value: "new", label: "New" }],
};
const chips = {
  Contacts: [owner, company],
  Companies: [owner],
  Deals: [owner, company, stage],
  Leads: [owner, status],
};

function Example({
  kind,
  options,
}: Readonly<{ kind: keyof typeof chips; options?: readonly ListChip[] }>) {
  const [query, setQuery] = useState("");
  const [chosen, setChosen] = useState<Record<string, string>>({});
  return (
    <ListSurface
      title={kind}
      search={{ value: query, onChange: setQuery }}
      chips={options ?? chips[kind]}
      chosen={chosen}
      onChipChange={(key, value) =>
        setChosen((before) => ({ ...before, [key]: value }))
      }
    >
      <p>List results</p>
    </ListSurface>
  );
}

const meta: Meta = {
  title: "Components/Text and data display/List search",
  component: ListSearch,
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

function openSuggestions(term: string, label: string) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const field = await within(canvasElement).findByRole("combobox", {
      name: "Search",
    });
    await userEvent.type(field, term);
    await expect(
      await within(document.body).findByRole("option", { name: label }),
    ).toBeVisible();
  };
}

export const Contacts: Story = {
  render: () => <Example kind="Contacts" />,
  play: openSuggestions("acme", "Company is Acme Engineering"),
};
export const Companies: Story = {
  render: () => <Example kind="Companies" />,
  play: openSuggestions("ada", "Owner is Ada"),
};
export const Deals: Story = {
  render: () => <Example kind="Deals" />,
  play: openSuggestions("prop", "Stage is Proposal"),
};
export const Leads: Story = {
  render: () => <Example kind="Leads" />,
  play: openSuggestions("new", "Status is New"),
};
export const KeyboardPick: Story = {
  render: () => <Example kind="Deals" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = await canvas.findByRole("combobox", { name: "Search" });
    await userEvent.type(field, "prop");
    await userEvent.keyboard("{ArrowDown}{Enter}");
    await expect(field).toHaveValue("");
    await expect(
      canvas.getByRole("group", { name: "Stage: Proposal" }),
    ).toBeVisible();
  },
};

const unavailableCompany: ListChip = {
  ...company,
  search: async () => {
    throw new Error("unavailable");
  },
};
export const FilterSearchFailed: Story = {
  render: () => <Example kind="Contacts" options={[unavailableCompany]} />,
  play: async ({ canvasElement }) => {
    const field = await within(canvasElement).findByRole("combobox", {
      name: "Search",
    });
    await userEvent.type(field, "acme");
    await expect(
      await within(document.body).findByRole("option", {
        name: /Filter suggestions did not load/,
      }),
    ).toBeVisible();
    await userEvent.keyboard("{Enter}");
    await expect(field).toHaveValue("acme");
  },
};
export const ContactsPhone: Story = {
  ...Contacts,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const ContactsDark: Story = {
  ...Contacts,
  globals: { theme: "dark" },
  tags: ["uat-dark"],
};
export const DealsPhone: Story = {
  ...Deals,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const DealsDark: Story = {
  ...Deals,
  globals: { theme: "dark" },
  tags: ["uat-dark"],
};
