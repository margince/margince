import type { Meta, StoryObj } from "@storybook/react-vite";
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
  title: "Records/Custom Details",
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
function routes(failed = false) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "GET /custom-fields": () =>
      failed
        ? jsonResponse({ title: "Unavailable" }, 503)
        : jsonResponse({
            data: [customFieldFixture],
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
export const ReadOnly: Story = {
  beforeEach: () => routes(),
  args: {
    kind: "contact",
    record: { id: "c1", version: 1, writable: false, cf_tier: "Strategic" },
  },
};
export const Failed: Story = {
  beforeEach: () => routes(true),
  args: { kind: "contact", record: { id: "c1", version: 1, writable: true } },
};
