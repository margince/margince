import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { briefModel, briefReady, meetingFacts, preparedFor } from "./fixtures";
import { BriefHeader } from "./header";
// The band's own sheet. The drawer reaches it through view.tsx; a story that
// renders the band alone reaches nothing, and `.mb-prepared-for` and
// `.mb-meeting-line` then draw as plain paragraphs — which is the one thing
// these stories exist to look at.
import "./meetingbrief.css";

// The brief's header band on its own: who it is prepared for, which meeting,
// and which writer produced the prose.
//
// The band is rendered from what the OPENING PAGE holds, not from the wire —
// the subject, the time and the contact are props — so a story here is a
// caller's knowledge rather than a server answer. `formatWhen` is the caller's
// too: this tier holds no locale and no zone, so the stories pass a fixed
// string rather than a live clock.

const meta: Meta<typeof BriefHeader> = {
  title: "Records/Contact record/Meeting brief/Header band",
  component: BriefHeader,
};
export default meta;
type Story = StoryObj<typeof BriefHeader>;

function band(brief: components["schemas"]["MeetingBrief"]) {
  return () => (
    <StoryProviders>
      <div className="drawer-head">
        <BriefHeader
          brief={brief}
          meeting={meetingFacts}
          preparedFor={preparedFor}
          formatWhen={() => "24 June, 15:00"}
        />
      </div>
    </StoryProviders>
  );
}

/** Everything a caller can hand over: the prepared-for line naming contact and
 *  company, the avatar beside the subject, the time under it, and the writer
 *  badge. */
export const Prepared: Story = { render: band(briefReady) };

/** The same band for prose a model wrote. Only the badge moves — indigo is a
 *  claim about who decided, and the facts above it are the same facts. */
export const ModelWritten: Story = { render: band(briefModel) };

/** The band in the dark theme: the prepared-for line and the time both sit on
 *  a muted role, and a muted role is where the dark accent lift shows. */
export const Dark: Story = {
  globals: { theme: "dark" },
  render: band(briefReady),
};

/** A caller that holds none of the meeting's facts — the deal page opens the
 *  drawer from an activity id alone. The band falls back to the badge rather
 *  than inventing a line, which is the state the two muted lines must not be
 *  drawn in. */
export const ActivityIdOnly: Story = {
  render: () => (
    <StoryProviders>
      <div className="drawer-head">
        <BriefHeader brief={briefReady} />
      </div>
    </StoryProviders>
  ),
};
