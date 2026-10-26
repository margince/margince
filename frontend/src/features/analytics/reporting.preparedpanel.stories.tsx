import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { PreparedReports } from "./reporting.preparedpanel";
import {
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";

const meta: Meta = { title: "Records/Reports/Analytics/Prepared reports" };
export default meta;
type Story = StoryObj;

// The ready reports a team lens is offered, each with its preview and copy.
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <PreparedReports scope={reportingStoryScope} ownLens={false} />
      </StoryProviders>
    );
  },
};

// A seat that sees only its own records also gets the reports about itself.
export const OwnLens: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <PreparedReports scope={reportingStoryScope} ownLens />
      </StoryProviders>
    );
  },
};
