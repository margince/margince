import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactMarks } from "./contactpage.marks";
import { recordedContact } from "./contactpage.marks.testkit";

const meta: Meta = { title: "Records/Contact 360/Identity marks" };
export default meta;
type Story = StoryObj;

function Marks() {
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
  return (
    <StoryProviders>
      <ContactMarks view={recordedContact} />
    </StoryProviders>
  );
}

export const LatestInbound: Story = { render: Marks };
export const LatestInboundDark: Story = {
  ...LatestInbound,
  globals: { theme: "dark" },
};
