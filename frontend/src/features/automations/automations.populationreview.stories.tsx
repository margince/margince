import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { configuredAutomation } from "./automations.fixtures";
import { ScheduledPopulationReview } from "./automations.populationreview";

const meta: Meta = {
  title: "Settings/Automations/Initial run",
};
export default meta;
type Story = StoryObj;

function Review({ fails = false }: Readonly<{ fails?: boolean }>) {
  const automation = configuredAutomation(
    "au-1",
    Date.parse("2026-09-01T12:00:00Z"),
  );
  installFetchStub({
    "POST /automations/au-1/preview": () =>
      fails
        ? jsonResponse({ title: "Preview failed", status: 500 }, 500)
        : jsonResponse({
            matches_now: 4,
            matches_now_available: true,
            window_days: 30,
          }),
  });
  return (
    <StoryProviders>
      <ScheduledPopulationReview
        automation={automation}
        pending={false}
        error={null}
        onConfirm={() => {}}
        onClose={() => {}}
      />
    </StoryProviders>
  );
}

export const MatchingRecords: Story = {
  render: () => <Review />,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByText("Matches now: 4")).toBeVisible();
  },
};
export const MatchingRecordsDark: Story = {
  ...MatchingRecords,
  globals: { theme: "dark" },
};
export const PreviewFailed: Story = { render: () => <Review fails /> };
