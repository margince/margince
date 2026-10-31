// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Disclosure } from "../../design-system/atoms";
import { Panel } from "../../design-system/panel";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { candidateVersion } from "../../testing/voice-candidate";
import {
  CandidateBanner,
  VoiceChangeLog,
  VoiceHistory,
} from "./voice-versions";

// Every build this profile has had, and what each one changed. A `candidate`
// is a finished build waiting for a human to accept or reject it — the state
// that makes this surface actionable rather than a log.
const SUPERSEDED = {
  id: "vv-2",
  profile_version: 2,
  status: "superseded",
  reason: "manual",
  source_count: 5,
  created_at: "2026-07-12T10:00:00Z",
};

const ACTIVE = {
  id: "vv-3",
  profile_version: 3,
  status: "active",
  reason: "automatic",
  source_count: 7,
  created_at: "2026-07-30T10:00:00Z",
};

const CANDIDATE = {
  id: "vv-4",
  profile_version: 4,
  status: "candidate",
  reason: "manual",
  source_count: 9,
  created_at: "2026-08-12T09:15:00Z",
};

const DELTA = {
  id: "vd-1",
  from_version: 3,
  to_version: 4,
  classification: "material",
  activation_outcome: "review_required",
  words_added: 2_140,
  created_at: "2026-08-12T09:15:00Z",
};

const LAST_PAGE = { next_cursor: null, has_more: false };

const LEARNING = {
  drafted: 6,
  accepted: 2,
  edited_sent: 3,
  rejected: 1,
  qualifying_source_count: 1,
  qualifying_words: 420,
  transformations: [],
};

function stub(
  versions: Record<string, unknown>[],
  deltas: Record<string, unknown>[],
) {
  installFetchStub({
    "GET /me": meRoute({ voice_profile: ["read", "update"] }),
    "GET /voice-profiles/vp-1/versions": () =>
      jsonResponse({ data: versions, page: LAST_PAGE }),
    "GET /voice-profiles/vp-1/deltas": () =>
      jsonResponse({ data: deltas, page: LAST_PAGE }),
    "GET /voice-profiles/vp-1/learning": () => jsonResponse(LEARNING),
  });
}

function story(
  versions: Record<string, unknown>[],
  canEdit: boolean,
  deltas: Record<string, unknown>[] = [DELTA],
) {
  return () => {
    stub(versions, deltas);
    return (
      <StoryProviders>
        <Panel title="Builds">
          <VoiceHistory
            profileId="vp-1"
            canEdit={canEdit}
            onChanged={() => undefined}
          />
          {/* Open here, closed in the product: a shut disclosure catalogues
              the chevron and nothing else. */}
          <Disclosure summary="What changed" open>
            <VoiceChangeLog profileId="vp-1" />
          </Disclosure>
        </Panel>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof VoiceHistory> = {
  title: "Settings/You/Writing voice/Build history",
  component: VoiceHistory,
};
export default meta;
type Story = StoryObj<typeof VoiceHistory>;

export const CandidateWaiting: Story = {
  render: story([CANDIDATE, ACTIVE, SUPERSEDED], true),
};

export const CandidateWaitingAtPhoneWidth: Story = {
  ...CandidateWaiting,
  tags: ["uat-phone"],
};

export const CandidateWaitingDark: Story = {
  ...CandidateWaiting,
  globals: { theme: "dark" },
};

// A rollback is one press away from replacing the active voice, so it asks.
export const RollbackConfirm: Story = {
  render: story([ACTIVE, SUPERSEDED], true),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for Version 2",
      }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      page.getByRole("button", { name: "Roll back to version 2" }),
    );
    await page.findByRole("dialog");
  },
};

export const OnlyActive: Story = {
  render: story([ACTIVE], true, []),
};

// The history is a read this seat keeps; accepting or rolling back a build is
// not. The rows stay and the verbs go.
export const ReadOnly: Story = {
  render: story([CANDIDATE, ACTIVE, SUPERSEDED], false),
};

export const TypedCandidateReasons: Story = {
  render: () => {
    const candidate = candidateVersion([
      "Recorded score explanation",
      "Recorded sample explanation",
    ]);
    candidate.evaluation.review_reason_details = [
      {
        code: "low_score",
        text: candidate.review_reasons[0] ?? "",
        values: { score: 0.56, floor: 0.6 },
      },
      {
        code: "corpus_changed",
        text: candidate.review_reasons[1] ?? "",
        values: {},
      },
    ];
    stub([candidate], []);
    return (
      <StoryProviders>
        <CandidateBanner
          profileId="vp-1"
          candidate={candidate}
          canEdit
          onChanged={() => undefined}
        />
      </StoryProviders>
    );
  },
};

export const TypedCandidateReasonsDark: Story = {
  ...TypedCandidateReasons,
  globals: { theme: "dark" },
};
