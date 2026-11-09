import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { meFixture } from "../../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
  storyQueryClient,
} from "../../../testing/story-utils";
import { systemTask } from "../../../testing/systemtext.fixture";
import { CloseTasksQuestion, useCloseTasks } from "./closetasks";

export default { title: "Patterns/Deal close tasks" } satisfies Meta;
function Question() {
  const state = useCloseTasks("demo-deal");
  return <CloseTasksQuestion state={state} />;
}
export const Vietnamese: StoryObj = {
  render: () => {
    installFetchStub({
      "GET /activities": () =>
        jsonResponse({
          data: [systemTask],
          page: { has_more: false, next_cursor: null },
        }),
    });
    const client = storyQueryClient();
    client.setQueryData(["me"], meFixture());
    return (
      <StoryProviders client={client} locale="vi">
        <Question />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText(
        systemTask.system_display?.subject ?? "",
      ),
    ).toBeVisible();
  },
};
