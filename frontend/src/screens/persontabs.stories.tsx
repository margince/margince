// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { PersonTimelineTab } from "./persontabs";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import "./person360.css";

// The contact's history tab: one chronology of what was said to them and what
// changed about them.
//
// The dials are what this file is for. The cuts (All / Activities / Changes)
// stood in the panel's head, where a row that wraps cannot stand — the head is
// one fixed band, and a second row of pills made this panel's title sit at a
// different height from every other panel on the page. They are in the body
// now, in the same `timeline-header` block the account and project pages
// already put them in: the cuts, then the row that narrows whichever cut is
// open.
//
// EVERY INSTANT IS FIXED, because `make fe-clock-drift` runs the suite at +200
// days and a relative date would re-group the rows under a different day.

type Person360 = components["schemas"]["Person360"];
type SectionActivity = NonNullable<Person360["activities"]>["data"][number];

const AT = "2026-08-13T09:00:00Z";

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: AT,
} as const;

function activity(
  row: Pick<SectionActivity, "id" | "kind" | "occurred_at"> &
    Partial<SectionActivity>,
): SectionActivity {
  return { is_done: false, ...CAPTURED, ...row };
}

// The tab reads the page's own activities section rather than fetching, so the
// rows below are the whole fixture: the change feed answers empty from the
// story stub, which is the honest state of a record nobody has edited.
const view: Person360 = {
  as_of: AT,
  person: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
  sections_omitted: [],
  activities: {
    data: [
      activity({
        id: "a-1",
        kind: "email",
        subject: "Fleet renewal",
        body: "Sending the retrofit numbers over before Thursday.",
        direction: "inbound",
        occurred_at: "2026-08-11T12:00:00Z",
      }),
      activity({
        id: "a-2",
        kind: "meeting",
        subject: "Depot walkthrough",
        body: "Walked the Hamburg depot with Dana and her workshop lead.",
        occurred_at: "2026-08-09T08:00:00Z",
      }),
    ],
    page: { has_more: false },
  },
};

// The tab is drawn at the work column's measure. The dials are the reason: how
// much of one line the cuts and the narrowing row take is a fact about the
// column they sit in, not about the viewport.
function tab(record: Person360 | undefined) {
  return () => {
    // The paged read asks the session which system of record this workspace
    // runs on — an overlay installation answers from the incumbent mirror and
    // drops dials the mirror refuses. Routed with no object grants, which is
    // the ordinary native seat this tab is drawn for; unrouted, the session
    // reads as malformed and the tab draws a branch these stories are not
    // named for.
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 760 }}>
          <PersonTimelineTab personId="p-1" view={record} />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PersonTimelineTab> = {
  title: "Records/Person record/History tab",
  component: PersonTimelineTab,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PersonTimelineTab>;

/**
 * The whole chronology, which is where a reader starts: the cuts under the
 * head with the narrowing row beneath them, and the exchanges under both.
 */
export const Chronology: Story = { render: tab(view) };

/**
 * The Conversations cut: the same chronicle, narrowed to the exchanges
 * somebody can answer, with the kind dial under it offering only the kinds
 * this cut can draw. Reached by pressing the pill, because the cut is the
 * reader's choice rather than a prop.
 */
export const Conversations: Story = {
  render: tab(view),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await page.findByRole("button", { name: "Conversations" }),
    );
  },
};

/**
 * The section the reader's grant does not reach. The dials stay — what they
 * cut is withheld, not absent, and a reader has to be able to ask for the
 * changes, which are a separate grant.
 */
export const Withheld: Story = {
  render: tab({
    ...view,
    activities: undefined,
    sections_omitted: ["activities"],
  }),
};

/**
 * Dark. Two rows of controls stacked in a panel body is where the pressed
 * pill's fill and the field chrome under it have to agree on one ground.
 */
export const ChronologyDark: Story = {
  ...Chronology,
  globals: { theme: "dark" },
};
