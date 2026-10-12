import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { reportingTargets } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { TargetHistory } from "./reporting.targethistory";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
export default {
  title: "Records/Reports/Analytics/Target history",
} satisfies Meta;
export const Retired: StoryObj = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByText("Target history", {
        selector: ".disclosure-label",
      }),
    );
  },
  render: () => {
    const [target] = reportingTargets;
    if (!target) {
      throw new Error("the reporting scenarios hold no target to draw");
    }
    installFetchStub({
      ...reportingStoryRoutes(),
      [`GET /analytics/targets/${target.id}`]: () =>
        jsonResponse({
          ...target,
          history: [
            target.definition,
            {
              ...target.definition,
              retired: true,
              reason: "Incorrect allocation retired",
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <TargetHistory target={target} />
      </StoryProviders>
    );
  },
};
