import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
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
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
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
