import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactToday } from "./contacttoday";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-08-13T09:00:00Z",
  contact: {
    id: "contact-1",
    full_name: "Dana Buyer",
    source: "referral",
    captured_by: "human:u-1",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
  },
  sections_omitted: [],
  privacy_notice_duties: [
    {
      id: "case-1",
      rule: "art14",
      due_at: "2026-08-12T09:00:00Z",
      blocked: false,
    },
  ],
};
const meta: Meta<typeof ContactToday> = {
  title: "Records/Contact 360/Next privacy duty",
  component: ContactToday,
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({ activity: ["read", "create"] }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: { view, onAction: () => undefined },
};
export default meta;
type Story = StoryObj<typeof ContactToday>;
export const PrivacyDuty: Story = {
  args: {
    moment: {
      rule: "privacy_notice_due",
      claim_key: "moment:privacy_notice:case-1",
      evidence_fingerprint: "case-1",
      headline: "Privacy notice owed",
      why_now: "A disclosure duty is due by 2026-08-12.",
      confidence: "observed_fact",
      evidence: [],
      recommended_action: {
        kind: "open_record",
        label: "Review privacy duty",
        state: "available",
        destination: { surface: "privacy_notice", entity_id: "case-1" },
      },
    },
  },
};
export const Quiet: Story = { args: { moment: undefined } };

// A seat without activity.create: the Next bar keeps its words and draws no
// verb that would draft a mail the server refuses.
export const DraftNotOfferedWithoutGrant: Story = {
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
      return <Story />;
    },
  ],
  args: {
    moment: {
      rule: "gone_quiet",
      claim_key: "moment:gone_quiet",
      evidence_fingerprint: "quiet-1",
      headline: "No reply recorded for 233 days",
      why_now: "The last message went out and nothing came back.",
      confidence: "observed_fact",
      evidence: [],
      recommended_action: {
        kind: "draft_reply",
        label: "Draft a follow-up",
        state: "available",
        destination: { surface: "composer", prefill: { intent: "follow_up" } },
      },
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("No reply recorded for 233 days");
    await waitFor(() =>
      expect(
        canvas.queryByRole("button", { name: "Draft a follow-up" }),
      ).toBeNull(),
    );
  },
};
