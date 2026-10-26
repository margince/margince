// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactTimelineTab } from "./contacttabs";
import "./contact360.css";

// The contact's history tab. The dials are what this file is for. The cuts (All / Threads / Changes) sit
// in the body, in the `timeline-header` block the account and project pages
// use. The head is one fixed band, and a wrapped row of pills there put this
// panel's title at a different height from every other panel.
//
// EVERY INSTANT IS FIXED, because `make fe-clock-drift` runs the suite at +200
// days and a relative date would re-group the rows under a different day.

type Contact360 = components["schemas"]["Contact360"];
type SectionActivity = NonNullable<Contact360["activities"]>["data"][number];

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
  return {
    occurred_precision: "instant",
    offers_email: false,
    is_done: false,
    ...CAPTURED,
    ...row,
  };
}

// The tab reads the page's own activities section rather than fetching, so the
// rows below are the whole fixture: the change feed answers empty from the
// story stub, which is the honest state of a record nobody has edited.
const view: Contact360 = {
  as_of: AT,
  contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
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
function tab(
  record: Contact360 | undefined,
  {
    width = 760,
    briefs = false,
    writer = false,
  }: { width?: number; briefs?: boolean; writer?: boolean } = {},
) {
  return () => {
    // Routed with no object grants, which is the ordinary seat this tab is
    // drawn for; unrouted, the session reads as malformed and the tab draws a
    // branch these stories are not named for.
    installFetchStub({
      "GET /me": meRoute(writer ? { activity: ["read", "update"] } : {}),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: width }}>
          <ContactTimelineTab
            contactId="p-1"
            view={record}
            onBriefMeeting={briefs ? () => undefined : undefined}
          />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ContactTimelineTab> = {
  title: "Records/Contact 360/History tab",
  component: ContactTimelineTab,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ContactTimelineTab>;

/**
 * The whole chronology, which is where a reader starts: the cuts under the
 * head with the narrowing row beneath them, and the exchanges under both.
 */
export const Chronology: Story = { render: tab(view) };

const reachable: Contact360["contact"] = {
  ...view.contact,
  phones: [
    {
      id: "ph-1",
      phone: "+49 151 7654321",
      phone_type: "mobile",
      is_primary: true,
      position: 0,
      ...CAPTURED,
    },
  ],
};

const call = activity({
  id: "a-3",
  kind: "call",
  subject: "Asked whether the retrofit slot can move to October",
  direction: "inbound",
  occurred_at: "2026-08-12T15:30:00Z",
});

/** A logged call on a contact with a phone offers Call on its row. */
export const WithCall: Story = {
  render: tab({
    ...view,
    contact: reachable,
    activities: {
      data: [call, ...(view.activities?.data ?? [])],
      page: { has_more: false },
    },
  }),
};

const LONG_SUBJECT =
  "Re: Retrofit schedule for the Hamburg depot, the October slot and the second workshop bay";

const LONG_WORD =
  "Flottenerneuerungsrahmenvertragsverhandlungsprotokollzusammenfassungsentwurfsüberarbeitungsfrist";
const LONG_FILENAME =
  "Rahmenvertrag_Flottenerneuerung_Hamburg_Depot_Werkstatt_2026_final_v3_unterschrieben_gegengezeichnet.pdf";

/**
 * Long words beside the verbs: an email subject, a call subject with no break
 * in it and a note named for a file. Each row's verbs stay inside the row.
 */
export const LongEmailBesideVerbs: Story = {
  render: tab({
    ...view,
    contact: reachable,
    activities: {
      data: [
        activity({
          id: "a-4",
          kind: "email",
          subject: LONG_SUBJECT,
          direction: "inbound",
          occurred_at: "2026-08-12T16:00:00Z",
          email_summary: {
            activity_id: "a-4",
            occurred_at: "2026-08-12T16:00:00Z",
            occurred_precision: "instant",
            version: 1,
            subject: LONG_SUBJECT,
            preview:
              "We can take the October slot if the second bay is free by the twentieth, otherwise it moves to November.",
            counterparty: "Dana Buyer",
            direction: "inbound",
            display_status: "team",
            move: "needs_reply",
            attachment_count: 0,
          },
        }),
        call,
        activity({
          id: "a-6",
          kind: "call",
          subject: LONG_WORD,
          direction: "outbound",
          occurred_at: "2026-08-12T14:00:00Z",
        }),
        activity({
          id: "a-7",
          kind: "note",
          subject: LONG_FILENAME,
          occurred_at: "2026-08-12T13:00:00Z",
        }),
        activity({
          id: "a-5",
          kind: "task",
          subject: "Send the revised retrofit quote",
          occurred_at: "2026-08-12T09:00:00Z",
        }),
        ...(view.activities?.data ?? []),
      ],
      page: { has_more: false },
    },
  }),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await page.findByText(LONG_SUBJECT);
    for (const body of canvasElement.querySelectorAll(".tl-body")) {
      const edge = body.getBoundingClientRect().right;
      for (const verb of body.querySelectorAll(":scope > .tl-actions > *")) {
        await expect(verb.getBoundingClientRect().right).toBeLessThanOrEqual(
          edge + 0.5,
        );
      }
    }
  },
};

/**
 * A held meeting carries five verbs, one more than any other row: Prepare
 * brief joins Reply, Files, Relink and Change visibility. Drawn just above the
 * width where the verbs drop under the entry, the kind line and the headline
 * stay clear of them.
 */
export const FiveVerbsBesideTheHead: Story = {
  render: tab(
    {
      ...view,
      contact: reachable,
      activities: {
        data: [
          activity({
            id: "a-8",
            kind: "meeting",
            subject: "Depot walkthrough with the workshop lead",
            body: "Walked the Hamburg depot with Dana and her workshop lead.",
            offers_email: true,
            writable: true,
            occurred_at: "2026-08-12T08:00:00Z",
          }),
        ],
        page: { has_more: false },
      },
    },
    { width: 680, briefs: true, writer: true },
  ),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await page.findByRole("button", { name: "Prepare brief" });
    for (const body of canvasElement.querySelectorAll(".tl-body")) {
      const head = body.querySelector(":scope > .tl-head");
      const verbs = body.querySelector(":scope > .tl-actions");
      if (!head || !verbs) continue;
      const row = verbs.getBoundingClientRect();
      // Each part of the kind line sits clear of the verbs. A collapsed head
      // still has a box, so its contents are measured instead.
      for (const part of head.children) {
        const box = part.getBoundingClientRect();
        await expect(
          box.right <= row.left + 0.5 || box.top >= row.bottom - 0.5,
        ).toBe(true);
      }
    }
  },
};

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
    await userEvent.click(await page.findByRole("button", { name: "Threads" }));
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
