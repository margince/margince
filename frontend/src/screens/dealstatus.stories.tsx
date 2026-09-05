import type { Meta, StoryObj } from "@storybook/react-vite";
import { DealStatusCardPanel } from "./dealstatus";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

const meta: Meta = { title: "Records/Deal next step" };
export default meta;
type Story = StoryObj;

export const ExistingTask: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "GET /deals/demo-deal/status": () =>
        jsonResponse({
          deal_id: "demo-deal",
          story: {
            sentences: [
              { text: "The buyer is reviewing the proposal.", evidence: [] },
            ],
          },
          verdict: { standing: "live", because: { sentences: [] } },
          next: {
            action: "open_task",
            reason: "Complete the existing task: Follow up on the proposal",
            arguments: { activity_id: "demo-task" },
            evidence: [
              { activity_id: "demo-task", text: "Follow up on the proposal" },
            ],
          },
          generated_at: "2026-09-05T09:00:00Z",
          generated_by: "model",
        }),
      "GET /deals/demo-deal/coverage": () =>
        jsonResponse({ stakeholders: [], risks: [] }),
    });
    return (
      <StoryProviders>
        <DealStatusCardPanel dealId="demo-deal" dealName="Demo proposal" />
      </StoryProviders>
    );
  },
};
