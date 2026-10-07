import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../api/schema";
import { RecordCustomFields } from "./recordcustomfields";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
export const customFieldFixture: components["schemas"]["CustomField"] = {
  id: "field1",
  object: "contact",
  label: "Customer tier",
  slug: "tier",
  type: "text",
  status: "active",
  column_name: "cf_tier",
  created_by: "u1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
const meta: Meta<typeof RecordCustomFields> = {
  title: "Records/Record 360/Custom details",
  excludeStories: ["customFieldFixture"],
  component: RecordCustomFields,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof RecordCustomFields>;
const FIELDS: components["schemas"]["CustomField"][] = [
  customFieldFixture,
  {
    ...customFieldFixture,
    id: "field2",
    label: "Annual budget",
    slug: "budget",
    type: "currency",
    currency: "EUR",
    column_name: "cf_budget",
  },
  {
    ...customFieldFixture,
    id: "field3",
    label: "Account wiki",
    slug: "wiki",
    column_name: "cf_wiki",
  },
];
// Wire-shaped values: a currency column travels as integer minor units.
const VALUES = {
  cf_tier: "Strategic",
  cf_budget: 4_800_000,
  cf_wiki: "https://wiki.example.com/globex",
};
function routes(failed = false) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "GET /custom-fields": () =>
      failed
        ? jsonResponse({ title: "Unavailable" }, 503)
        : jsonResponse({
            data: FIELDS,
            page: { has_more: false },
          }),
    "PATCH /contacts/c1": (body) =>
      jsonResponse({
        id: "c1",
        version: 2,
        ...(typeof body === "object" ? body : {}),
      }),
  });
}
export const Unset: Story = {
  beforeEach: () => routes(),
  args: { kind: "contact", record: { id: "c1", version: 1, writable: true } },
};
export const Filled: Story = {
  beforeEach: () => routes(),
  args: {
    kind: "contact",
    record: { id: "c1", version: 1, writable: true, ...VALUES },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: VALUES.cf_wiki }),
    ).toHaveAttribute("href", VALUES.cf_wiki);
    await expect(
      canvas.getByRole("button", { name: "Change Annual budget" }),
    ).toHaveTextContent("€48,000.00");
  },
};
export const ReadOnly: Story = {
  beforeEach: () => routes(),
  args: {
    kind: "contact",
    record: { id: "c1", version: 1, writable: false, ...VALUES },
  },
};
export const Failed: Story = {
  beforeEach: () => routes(true),
  args: { kind: "contact", record: { id: "c1", version: 1, writable: true } },
};
