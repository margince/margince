import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactPageV2 } from "./contactpage";
import { page, populated, type View } from "./contactpage.fixtures";
import "./contact360.css";

// The contact's Companies tab on the whole page: where they work and worked,
// drawn as one timeline, and the same tab for a contact with no company.

const meta: Meta = { title: "Records/Contact 360/Companies tab" };
export default meta;
type Story = StoryObj;

function formerJob(
  id: string,
  name: string,
  role: string,
  from: number,
  to: number,
) {
  return {
    relationship_id: `rel-${id}`,
    company_id: id,
    company_name: name,
    role,
    is_current_primary: false,
    started_at: `${from}-01-01T00:00:00Z`,
    ended_at: `${to}-01-01T00:00:00Z`,
  };
}

const career: View = {
  ...populated,
  employments: {
    data: [
      ...(populated.employments?.data ?? []),
      formerJob("o-2", "Voss Logistics", "Fleet Coordinator", 2018, 2022),
      formerJob("o-3", "Kessler Spedition", "Dispatcher", 2014, 2018),
    ],
    page,
  },
};

const unemployed: View = { ...populated, employments: { data: [], page } };

function CompaniesTab({ view }: Readonly<{ view: View }>) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }),
    "GET /contacts/p-1": () => jsonResponse(view.contact),
    "GET /contacts/p-1/360": () => jsonResponse(view),
    "GET /contacts/p-1/brief": () =>
      jsonResponse({ contact_id: "p-1", sentences: [], generated_by: "rules" }),
    "GET /contacts/p-1/consent/guard": () =>
      jsonResponse({ contact_id: "p-1", entries: [] }),
    "GET /channel-providers": () => jsonResponse({ data: [] }),
  });
  return (
    <StoryProviders>
      <ContactPageV2 id="p-1" tab="companies" />
    </StoryProviders>
  );
}

export const WorkHistory: Story = {
  render: () => <CompaniesTab view={career} />,
};

export const WorkHistoryDark: Story = {
  ...WorkHistory,
  globals: { theme: "dark" },
};

export const NoCompany: Story = {
  render: () => <CompaniesTab view={unemployed} />,
};
