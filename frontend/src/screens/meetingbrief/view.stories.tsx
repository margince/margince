import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { briefEmpty, briefReady, meetingFacts, preparedFor } from "./fixtures";
import { type BriefViewState, MeetingBriefView } from "./view";
// `.pe-disclosure` in the foot belongs to the contact record's sheet, which the
// drawer reaches through the page around it. A story mounting the body alone
// reaches neither, and the assembled-now line then draws at body size.
import "../contact360.css";

// The brief's body as a pure component: state in, prose out, no fetching. The
// connected drawer's stories (Records/Contact record/Meeting brief) cover the
// read; these cover the four states the body itself can be in, from a fixture.
//
// Every date here is fixed rather than relative: the suite runs at +200 days
// under `make fe-clock-drift` and must reach the same verdict there, so
// `formatWhen` returns a constant instead of reading a clock.

const meta: Meta<typeof MeetingBriefView> = {
  title: "Records/Contact record/Meeting brief/Body",
  component: MeetingBriefView,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof MeetingBriefView>;

function body(state: BriefViewState) {
  return () => (
    <StoryProviders>
      <MeetingBriefView
        state={state}
        meeting={meetingFacts}
        preparedFor={preparedFor}
        onOpenRecord={() => {}}
        titleId="meeting-brief-title"
        onClose={() => {}}
        formatWhen={() => "24 June, 15:00"}
        formatDay={(iso) => iso.slice(0, 10)}
      />
    </StoryProviders>
  );
}

/** The prepared brief: the header band, the nine sections in the hierarchy a
 *  reader two minutes from a room needs, and the foot's disclosure that the
 *  prose was assembled just now. */
export const Prepared: Story = {
  render: body({ kind: "ready", brief: briefReady }),
};

/** A cold record. Not an error — the foot still says when the brief was
 *  assembled, because a reader has to be able to tell "nothing recorded" from
 *  "not read yet". */
export const NothingRecorded: Story = {
  render: body({ kind: "ready", brief: briefEmpty }),
};

/** The read failed. The generic sentence carries the retry and the server's own
 *  sentence sits under it, so a reason a reader can act on is not thrown away. */
export const Failed: Story = {
  render: body({
    kind: "failed",
    message: "That meeting is filed under a different engagement.",
    onRetry: () => {},
  }),
};

/** The same body in the dark theme: the foot's disclosure is the smallest role
 *  on the surface, and the smallest role is where the dark lift shows first. */
export const PreparedDark: Story = {
  globals: { theme: "dark" },
  render: body({ kind: "ready", brief: briefReady }),
};
