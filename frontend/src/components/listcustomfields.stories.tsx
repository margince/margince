import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import type { CustomField } from "./customfields.form";
import { type ListQuery, ListTable } from "./listquery";
import { customFieldFixture } from "./recordcustomfields.stories";

export const LIST_FIELDS: CustomField[] = [
  {
    ...customFieldFixture,
    label: "Customer tier",
    type: "picklist",
    options: ["Gold", "Silver"],
  },
  {
    ...customFieldFixture,
    id: "budget",
    label: "Annual budget",
    column_name: "cf_budget",
    slug: "budget",
    type: "currency",
    currency: "EUR",
  },
  {
    ...customFieldFixture,
    id: "choices",
    label: "Capabilities",
    column_name: "cf_capabilities",
    slug: "capabilities",
    type: "multiselect",
    options: ["Fit, scope", "C++"],
  },
  {
    ...customFieldFixture,
    id: "retired",
    label: "Retired field",
    column_name: "cf_retired",
    slug: "retired",
    status: "retired",
  },
];

export function CustomFieldListExample({
  masked = false,
}: Readonly<{ masked?: boolean }>) {
  const [query, setQuery] = useState<ListQuery>({
    q: "",
    sort: "",
    filters: {},
    includeArchived: false,
    perPage: 25,
  });
  const row = {
    id: "contact1",
    name: "Dana Buyer",
    cf_tier: "Gold",
    cf_budget: 12500,
    cf_capabilities: ["C++", "Fit, scope"],
    masked_fields: masked ? ["cf_budget"] : [],
  };
  return (
    <ListTable
      customFieldsKind="contact"
      columns={[
        { key: "name", header: "Name", fixed: true, cell: (r) => r.name },
      ]}
      rowKey={(r) => r.id}
      unit="contacts"
      state={{
        rows: [row],
        query,
        setQuery,
        isPending: false,
        isError: false,
        error: null,
        refetch: () => {},
        hasMore: false,
        loadMore: () => {},
      }}
    />
  );
}

const meta: Meta<typeof CustomFieldListExample> = {
  title: "Components/Lists/Custom fields",
  component: CustomFieldListExample,
  excludeStories: ["LIST_FIELDS", "CustomFieldListExample"],
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }),
      "GET /custom-fields": () =>
        jsonResponse({ data: LIST_FIELDS, page: { has_more: false } }),
    });
  },
};
export default meta;
type Story = StoryObj<typeof CustomFieldListExample>;
export const OptionalColumnsAndFilters: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Dana Buyer");
    await userEvent.click(canvas.getByRole("button", { name: "View" }));
    const dialog = within(document.body);
    await userEvent.click(
      await dialog.findByRole("button", { name: /Columns/ }),
    );
    await expect(
      dialog.getByRole("checkbox", { name: "Customer tier" }),
    ).toBeInTheDocument();
  },
};
export const MaskedBudget: Story = { args: { masked: true } };
