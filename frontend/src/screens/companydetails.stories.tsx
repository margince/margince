import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyDetails } from "./companydetails";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const meta: Meta<typeof CompanyDetails> = {
  title: "Records/Company Details",
  component: CompanyDetails,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () =>
    installFetchStub({
      "GET /me": meRoute({ company: ["read", "update"] }, { seat: "full" }),
      "GET /users": () => jsonResponse({ data: [] }),
    }),
};
export default meta;
type Story = StoryObj<typeof CompanyDetails>;
const fixture: components["schemas"]["Company"] = {
  id: "co1",
  display_name: "Brandt Automotive",
  writable: true,
  version: 1,
  lifecycle: "prospect",
  domains: [
    {
      id: "domain1",
      domain: "brandt.example",
      is_primary: true,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  address: { city: "Munich", country: "DE" },
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
export const Editable: Story = {
  args: {
    overlay: false,
    company: fixture,
  },
};
export const ReadOnly: Story = {
  args: {
    overlay: false,
    company: {
      ...fixture,
      id: "co1",
      display_name: "Brandt Automotive",
      writable: false,
    },
  },
};
