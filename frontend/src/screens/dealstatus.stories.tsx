import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DealStatusCardPanel } from "./dealstatus";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type WrittenBy = components["schemas"]["WrittenBy"];
type Situation = "task" | "request" | "covered";

function panel(writer: WrittenBy, situation: Situation = "task") {
  const request = situation !== "task";
  const next = request
    ? {
        action: situation === "covered" ? "draft_email" : "create_task",
        reason:
          situation === "covered"
            ? "This request already has a reminder. The reply still needs to be handled."
            : "Review and take responsibility for the outstanding request: Meeting slots",
        arguments:
          situation === "covered"
            ? {}
            : {
                subject: "Meeting slots",
                source: "ui",
                request_activity_id: "demo-mail",
              },
        evidence: [
          {
            activity_id: "demo-mail",
            text: "Yes, please send a few meeting slots.",
          },
        ],
      }
    : {
        action: "open_task",
        reason: "Complete the existing task: Follow up on the proposal",
        arguments: { activity_id: "demo-task" },
        evidence: [
          { activity_id: "demo-task", text: "Follow up on the proposal" },
        ],
      };
  return () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create", "update"] }),
      "GET /deals/demo-deal/status": () =>
        jsonResponse({
          deal_id: "demo-deal",
          story: {
            sentences: [
              {
                text: request
                  ? "This deal is won. The customer asked for meeting slots."
                  : "The buyer is reviewing the proposal.",
                evidence: [],
              },
            ],
          },
          verdict: { standing: "live", because: { sentences: [] } },
          next,
          generated_at: "2026-09-05T09:00:00Z",
          generated_by: writer,
        }),
      "GET /deals/demo-deal/coverage": () =>
        jsonResponse({ stakeholders: [], risks: [] }),
    });
    return (
      <StoryProviders>
        <DealStatusCardPanel
          dealId="demo-deal"
          dealName={request ? "Won account" : "Demo proposal"}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta = { title: "Records/Deal next step" };
export default meta;
type Story = StoryObj;

export const ExistingTask: Story = { render: panel("model") };
export const ComposedBrief: Story = { render: panel("deterministic") };
export const ExistingTaskDark: Story = {
  globals: { theme: "dark" },
  render: panel("model"),
};
export const HistoricalRequestOnWonDeal: Story = {
  render: panel("deterministic", "request"),
};
export const HistoricalRequestOnWonDealDark: Story = {
  ...HistoricalRequestOnWonDeal,
  globals: { theme: "dark" },
};
export const RequestAlreadyCovered: Story = {
  render: panel("deterministic", "covered"),
};
export const RequestAlreadyCoveredDark: Story = {
  ...RequestAlreadyCovered,
  globals: { theme: "dark" },
};
