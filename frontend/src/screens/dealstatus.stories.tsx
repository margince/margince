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

// The model's own prose. The brief's band is indigo and its head discloses
// itself; the foot names the writer beside "Write it again", the machine's own
// verb — quiet, because it sits inside the panel that writer already filled.
export const ExistingTask: Story = { render: panel("model") };

// The same read with no model lane behind it. STILL indigo, because the
// briefing is the machine's reading either way; the only thing that moves is
// the foot, which now says the words were assembled from the records.
export const ComposedBrief: Story = { render: panel("deterministic") };

// The indigo head band and the quiet indigo verb in the dark theme: `--aiText`
// on `--aiLight` is the pair the dark accent lift moves first.
export const ExistingTaskDark: Story = {
  globals: { theme: "dark" },
  render: panel("model"),
};
