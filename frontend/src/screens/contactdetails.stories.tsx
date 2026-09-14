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
  title: "Records/Contact Details",
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
