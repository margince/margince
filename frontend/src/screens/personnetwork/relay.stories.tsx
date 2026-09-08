// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { IntroRequest } from "../introrequests";
import { StoryProviders } from "../story-utils";
import { RelayPanel } from "./relay";
import "../personnetwork.css";

// Where an introduction has got to, and who owes the next move.
//
// The four steps are the whole gallery: the numeral in each marker is the
// smallest text on the panel, and the ring around it is COLOUR — so a story
// has to show a done, a current and a pending step at once, or the state that
// reads only as a tint goes unchecked. The word beside each step is what
// actually carries it.
//
// Dates are fixed rather than relative: `make fe-clock-drift` runs the suite at
// +200 days and must reach the same verdict.

const REQUESTER = "018f3a1b-0000-7000-8000-000000000001";
const INTRODUCER = "018f3a1b-0000-7000-8000-000000000021";
const PERSON = "018f3a1b-0000-7000-8000-000000000010";

// One ask, moved along by status and the timestamps the server sets with it.
// The relay reads the timestamps rather than the status alone, so a fixture
// that advanced one without the other would draw a ladder the wire cannot
// produce.
function ask(over: Partial<IntroRequest> = {}): IntroRequest {
  return {
    id: "018f3a1b-0000-7000-8000-0000000000a1",
    person_id: PERSON,
    requester_user_id: REQUESTER,
    requester_display_name: "Demo Admin",
    introducer_user_id: INTRODUCER,
    introducer_display_name: "Sofia Meier",
    route_type: "direct",
    status: "requested",
    internal_reason: "Dana reopened the retrofit conversation after 41 days.",
    name_drop_allowed: false,
    note_generated_by: "human",
    note_ai_generated: false,
    fallback_policy: "none",
    requested_at: "2026-08-30T08:00:00Z",
    due_at: "2026-09-06T08:00:00Z",
    version: 1,
    ...over,
  };
}

const meta: Meta<typeof RelayPanel> = {
  title: "Records/Person/Introduction relay",
  component: RelayPanel,
};
export default meta;
type Story = StoryObj<typeof RelayPanel>;

function relay(request: IntroRequest | undefined) {
  return () => (
    <StoryProviders>
      <RelayPanel ask={request} />
    </StoryProviders>
  );
}

/** Nothing asked for yet: step one is current and the three behind it are
 *  pending, so this is the story that shows a numeral with no ring filled. */
export const NoAskYet: Story = { render: relay(undefined) };

/** Waiting on the colleague. Step one is done and carries a check instead of
 *  its numeral, step two is current, and the owner line under the ladder names
 *  who owes the move with the answer window beside it. */
export const AwaitingAnswer: Story = { render: relay(ask()) };

/** The handshake happened. Three checks and one pending step — the reply is
 *  observed from captured activity, so nothing here can declare it. */
export const Introduced: Story = {
  render: relay(
    ask({
      status: "introduced",
      decided_at: "2026-08-31T09:00:00Z",
      introduced_at: "2026-09-01T10:00:00Z",
    }),
  ),
};

/** Permission to mention a name, used. The third step says NAME-DROP rather
 *  than introduction: the two are different events, and a ladder that showed
 *  lent permission as a completed handshake would be the lie this panel exists
 *  to avoid. */
export const NameDropped: Story = {
  render: relay(
    ask({
      status: "name_dropped",
      name_drop_allowed: true,
      decided_at: "2026-08-31T09:00:00Z",
      name_dropped_at: "2026-09-01T10:00:00Z",
    }),
  ),
};

/** The contact answered: four done steps, and an owner line that names nobody
 *  rather than pointing at a colleague who has already done their part. The
 *  answer window is gone with it — a due date belongs only to the step that is
 *  still waiting on somebody. */
export const Replied: Story = {
  render: relay(
    ask({
      status: "replied",
      decided_at: "2026-08-31T09:00:00Z",
      introduced_at: "2026-09-01T10:00:00Z",
      replied_at: "2026-09-03T07:30:00Z",
    }),
  ),
};

/** The waiting ladder in the dark theme, where the pending marker's ring and
 *  its numeral are the closest pair of roles on the panel. */
export const AwaitingAnswerDark: Story = {
  globals: { theme: "dark" },
  render: relay(ask()),
};
