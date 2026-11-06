import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { GoalFields, goalOf } from "./reporting.initiativegoal";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Initiative goal",
  beforeEach: () => installFetchStub(reportingStoryRoutes()),
};
export default meta;
type Story = StoryObj;
function Goal() {
  const [goal, setGoal] = useState(goalOf());
  return (
    <GoalFields goal={goal} onChange={setGoal} onProposal={() => undefined} />
  );
}
export const Default: Story = {
  render: () => (
    <StoryProviders>
      <Goal />
    </StoryProviders>
  ),
};
export const Dark: Story = { ...Default, globals: { theme: "dark" } };
