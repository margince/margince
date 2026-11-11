import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { configuredAutomation } from "./automations.fixtures";
import { AutomationStatus } from "./automations.status";

const meta: Meta = { title: "Settings/Automations/Status" };
export default meta;
type Story = StoryObj;

function ScheduledStatus() {
  const automation: components["schemas"]["Automation"] = {
    ...configuredAutomation("au-1", Date.parse("2026-09-01T12:00:00Z")),
    status: "paused",
    initial_run_preview_required: true,
  };
  installFetchStub({
    "POST /automations/au-1/preview": () =>
      jsonResponse({
        matches_now: 4,
        matches_now_available: true,
        window_days: 30,
      }),
  });
  return (
    <StoryProviders>
      <AutomationStatus automation={automation} canEdit />
    </StoryProviders>
  );
}

export const ReviewBeforeEnable: Story = {
  render: () => <ScheduledStatus />,
  play: async ({ canvasElement }) => {
    await userEvent.click(await within(canvasElement).findByRole("switch"));
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByText("Matches now: 4")).toBeVisible();
  },
};
export const ReviewBeforeEnableDark: Story = {
  ...ReviewBeforeEnable,
  globals: { theme: "dark" },
};
