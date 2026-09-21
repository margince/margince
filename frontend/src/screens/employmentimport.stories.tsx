import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ImportedEmploymentHistory } from "./employmentimport";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-09-01T00:00:00Z",
  sections_omitted: [],
  contact: {
    id: "sample",
    full_name: "Sample Contact",
    source: "manual",
    captured_by: "human:sample",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  },
  provider_profiles: [
    {
      provider: "surfe",
      state: "completed",
      categories_not_requested: [],
      emails: [],
      mobile_phones: [],
      departments: [],
      seniorities: [],
      job_history: [{ company_name: "History Co", job_title: "Advisor" }],
    },
  ],
};
const items: components["schemas"]["EmploymentImportItem"][] = [
  {
    key: "one",
    company_name: "History Co",
    role: "Advisor",
    provider: "surfe",
    employment_status: "unknown",
    state: "needs_match",
    started: "2020-02",
  },
  {
    key: "two",
    company_name: "History Co",
    role: "Engineer",
    provider: "surfe",
    employment_status: "former",
    state: "needs_match",
    started: "2015-01",
    ended: "2019-12",
  },
];
const meta: Meta<typeof ImportedEmploymentHistory> = {
  title: "Records/Contact record/Imported employment",
  component: ImportedEmploymentHistory,
  args: { view, canEdit: true },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({
          contact: ["read", "update"],
          company: ["read"],
          relationship: ["read", "create"],
        }),
        "GET /contacts/sample/employment-import": () =>
          jsonResponse({ contact_id: "sample", items }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof meta>;
export const Unresolved: Story = {};
export const ReadOnly: Story = { args: { canEdit: false } };
