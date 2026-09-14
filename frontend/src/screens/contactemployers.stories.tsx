import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Employers } from "./contactemployers";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import "./contact360.css";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-09-01T00:00:00Z",
  sections_omitted: [],
  contact: {
    id: "sample",
    full_name: "Sample Contact",
    writable: true,
    source: "manual",
    captured_by: "human:sample",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  },
  employments: {
    data: [
      {
        relationship_id: "current",
        company_id: "one",
        company_name: "Current Company",
        role: "Lead",
        is_current_primary: true,
        employment_status: "current",
        version: 1,
      },
      {
        relationship_id: "former",
        company_id: "two",
        company_name: "Earlier Company",
        role: "Engineer",
        is_current_primary: false,
        employment_status: "former",
        started_at: "2020-02-01",
        ended_at: "2023-04-01",
        started_precision: "month",
        ended_precision: "month",
        version: 1,
      },
      {
        relationship_id: "unknown",
        company_id: "two",
        company_name: "Earlier Company",
        role: "Advisor",
        is_current_primary: false,
        employment_status: "unknown",
        version: 1,
      },
    ],
    page: { has_more: false },
  },
};
const meta: Meta<typeof Employers> = {
  title: "Records/Contact record/Employment history",
  component: Employers,
  args: { view },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({
          contact: ["read", "update"],
          relationship: ["read", "update", "create", "delete"],
          company: ["read"],
        }),
      });
      return (
        <StoryProviders>
          <div style={{ maxWidth: 420 }}>
            <Story />
          </div>
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof meta>;
export const GroupedRoles: Story = {};
export const ReadOnly: Story = {
  args: { view: { ...view, contact: { ...view.contact, writable: false } } },
};
