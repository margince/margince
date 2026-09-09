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

// One helper rather than a stub literal per story: what varies between these
// frames is WHO wrote the briefing, and spelling the two routes out twice
// invites them to drift apart.
function panel(writer: WrittenBy) {
  return () => {
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
          generated_by: writer,
        }),
      "GET /deals/demo-deal/coverage": () =>
        jsonResponse({ stakeholders: [], risks: [] }),
    });
    return (
      <StoryProviders>
        <DealStatusCardPanel dealId="demo-deal" dealName="Demo proposal" />
      </StoryProviders>
    );
  };
}

const meta: Meta = { title: "Records/Deal next step" };
export default meta;
type Story = StoryObj;

// The model's own reading. The brief's band goes indigo, the badge names the
// writer, and "Write it again" is the machine's verb — quiet, because it sits
// inside the panel the machine already wrote.
export const ExistingTask: Story = { render: panel("model") };

// The same read with no model lane behind it. The briefing is still written and
// still cited; nothing is indigo, because indigo is a claim about authorship
// and a deterministic composition would be borrowing it.
export const ComposedBrief: Story = { render: panel("deterministic") };

// The indigo band and the quiet indigo verb in the dark theme: `--aiText` on
// `--aiLight` is the pair the dark accent lift moves first.
export const ExistingTaskDark: Story = {
  globals: { theme: "dark" },
  render: panel("model"),
};
