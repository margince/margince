import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ContactDetails } from "./contactdetails";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const meta: Meta<typeof ContactDetails> = {
  title: "Records/Contact 360/Details",
  component: ContactDetails,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () =>
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "GET /users": () => jsonResponse({ data: [] }),
    }),
};
export default meta;
type Story = StoryObj<typeof ContactDetails>;
const fixture: components["schemas"]["Contact"] = {
  id: "c1",
  full_name: "Dana Buyer",
  title: "Director",
  writable: true,
  version: 1,
  visibility: "workspace",
  emails: [],
  phones: [],
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};
export const Editable: Story = {
  args: {
    contact: fixture,
  },
};
export const ReadOnly: Story = {
  args: {
    contact: {
      ...fixture,
      id: "c1",
      full_name: "Dana Buyer",
      writable: false,
    },
  },
};

// A contact a purchase filled: the title, profile link, one address and one
// number carry the "bought" mark beside them, the typed address does not, and
// the location and departments the provider sold stand in their own row.
const boughtFixture: components["schemas"]["Contact"] = {
  ...fixture,
  title: "Head of Revenue",
  social: { linkedin: "https://www.linkedin.com/in/dana" },
  emails: [
    {
      id: "e-bought",
      email: "dana@bought.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "surfe",
      captured_by: "connector:surfe",
    },
    {
      id: "e-typed",
      email: "dana@typed.example",
      email_type: "personal",
      is_primary: false,
      position: 1,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  bought_fields: [
    { target: "title", provider: "surfe", applied_at: "2026-06-02T12:00:00Z" },
    {
      target: "linkedin",
      provider: "surfe",
      applied_at: "2026-06-02T12:00:00Z",
    },
    {
      target: "email:e-bought",
      provider: "surfe",
      applied_at: "2026-06-02T12:00:00Z",
    },
  ],
};
const boughtProfiles: components["schemas"]["ContactProviderProfile"][] = [
  {
    provider: "surfe",
    state: "completed",
    categories_not_requested: [],
    emails: [],
    mobile_phones: [],
    job_history: [],
    departments: ["Sales"],
    seniorities: [],
    location: "Munich, Germany",
    attributes: [
      {
        kind: "location",
        value: "Munich, Germany",
        retrieved_at: "2026-06-02T12:00:00Z",
      },
      {
        kind: "department",
        value: "Sales",
        retrieved_at: "2026-03-02T12:00:00Z",
      },
    ],
  },
];
export const BoughtValues: Story = {
  args: { contact: boughtFixture, profiles: boughtProfiles },
};
export const BoughtValuesDark: Story = {
  args: { contact: boughtFixture, profiles: boughtProfiles },
  globals: { theme: "dark" },
};
