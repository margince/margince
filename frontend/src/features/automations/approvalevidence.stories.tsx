import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ApprovalEvidence } from "./approvalevidence";

const meta = {
  title: "Patterns/Approval evidence",
  component: ApprovalEvidence,
} satisfies Meta<typeof ApprovalEvidence>;
export default meta;
const source = "22222222-2222-4222-8222-222222222222";
const proposal = {
  id: "approval",
  kind: "commitment_task",
  status: "pending",
  proposed_by: "system:transcript",
  created_at: "2026-09-01T09:00:00Z",
} as const;

export const SourceNotRecorded: StoryObj = {
  render: () => (
    <StoryProviders>
      <ApprovalEvidence approval={proposal} />
    </StoryProviders>
  ),
};
export const SourceUnavailable: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${source}`]: () =>
        jsonResponse({ code: "not_found" }, 404),
    });
    return (
      <StoryProviders>
        <ApprovalEvidence
          approval={{
            ...proposal,
            target_entity_type: "activity",
            target_entity_id: source,
          }}
        />
      </StoryProviders>
    );
  },
};

export const LifecycleMeeting: StoryObj = {
  render: () => {
    installFetchStub({
      [`GET /activities/${source}`]: () =>
        jsonResponse({
          id: source,
          kind: "meeting",
          subject: "Contract review",
          body: "The buyer ended the agreement.",
          occurred_at: "2026-09-01T09:00:00Z",
          is_done: true,
          source: "manual",
          captured_by: "human:reviewer",
          created_at: "2026-09-01T09:00:00Z",
          updated_at: "2026-09-01T09:00:00Z",
          links: [],
          version: 1,
        }),
    });
    return (
      <StoryProviders>
        <ApprovalEvidence
          approval={{
            ...proposal,
            kind: "lifecycle_change",
            target_entity_type: "company",
            target_entity_id: "company",
            preview: {
              version: 1,
              state: "ready",
              changes: [],
              reviewer_reason: "record_authority",
              evidence: [
                {
                  source_type: "activity",
                  source_id: source,
                  evidence_snippet: "The buyer ended the agreement.",
                },
              ],
            },
          }}
        />
      </StoryProviders>
    );
  },
};
