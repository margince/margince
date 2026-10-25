import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../../design-system/panel";
import { viewerZone } from "../../format/timezone";
import { wonProposal } from "../../testing/magic.approval.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { MagicLaneSection } from "./magic.lines";

const meta: Meta<typeof MagicLaneSection> = {
  title: "Shell/Home/Approval context",
  component: MagicLaneSection,
};
export default meta;
type Story = StoryObj<typeof MagicLaneSection>;
export const Ready: Story = {
  render: () => (
    <StoryProviders>
      <Panel>
        <MagicLaneSection
          lane="needs_you"
          title="Waiting on you"
          rows={[wonProposal]}
          since={undefined}
          zone={viewerZone()}
          byDay={false}
        />
      </Panel>
    </StoryProviders>
  ),
};
export const ReadyDark: Story = { ...Ready, globals: { theme: "dark" } };
export const Stale: Story = {
  render: () => (
    <StoryProviders>
      <Panel>
        <MagicLaneSection
          lane="needs_you"
          title="Waiting on you"
          rows={[
            {
              ...wonProposal,
              approval_preview: {
                version: 2,
                state: "stale",
                reviewer_reason: "requested_decider",
                changes: [],
              },
            },
          ]}
          since={undefined}
          zone={viewerZone()}
          byDay={false}
        />
      </Panel>
    </StoryProviders>
  ),
};

export const WithoutOptionalEvidence: Story = {
  render: () => (
    <StoryProviders>
      <Panel>
        <MagicLaneSection
          lane="needs_you"
          title="Waiting on you"
          rows={[
            {
              ...wonProposal,
              approval_preview: wonProposal.approval_preview && {
                ...wonProposal.approval_preview,
                evidence: [],
              },
            },
          ]}
          since={undefined}
          zone={viewerZone()}
          byDay={false}
        />
      </Panel>
    </StoryProviders>
  ),
};

// The account's ending cites a meeting logged for its date alone: the source
// date is that date on the record's clock, with no time.
export const DayLifecycleSource: Story = {
  render: () => (
    <StoryProviders>
      <Panel>
        <MagicLaneSection
          lane="needs_you"
          title="Waiting on you"
          rows={[
            {
              ...wonProposal,
              approval_preview: wonProposal.approval_preview && {
                ...wonProposal.approval_preview,
                lifecycle_evidence: {
                  scope: "company",
                  occurred_at: "2026-07-31T12:00:00Z",
                  occurred_precision: "day",
                },
              },
            },
          ]}
          since={undefined}
          zone={viewerZone()}
          byDay={false}
        />
      </Panel>
    </StoryProviders>
  ),
};
