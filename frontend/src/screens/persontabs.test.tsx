/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import {
  PersonDealsTab,
  PersonMeetingsTab,
  PersonTimelineTab,
} from "./persontabs";
import { stubWithSession } from "./story-utils";

type Person360 = components["schemas"]["Person360"];
// The 360 carries its OWN spelling of an activity row — the section's element
// type, not the standalone `Activity` schema, which differs in two fields.
// Deriving it from the section is what keeps this fixture honest about the
// payload the page actually reads.
type SectionActivity = NonNullable<Person360["activities"]>["data"][number];

// Provenance is stamped by the server on every captured row, so a fixture
// that leaves it out is a payload no reader ever receives. These two build a
// row the CONTRACT admits, which is what makes the fixtures below typed
// rather than asserted: an assertion would let a field the page reads drift
// out of the fixture without a word from the compiler.
const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

function activity(
  row: Pick<SectionActivity, "id" | "kind" | "occurred_at"> &
    Partial<SectionActivity>,
): SectionActivity {
  return { is_done: false, ...CAPTURED, ...row };
}

// The six tabs beside Overview were placeholders for a release, and the thing
// that made them safe to ship as placeholders — a sentence saying so — is
// exactly what makes a regression invisible: a tab that silently renders
// nothing looks like a tab with nothing on it. These pin the two facts a
// reader acts on. That a section WITH rows draws them, and that a section the
// grant withheld says so rather than reading as empty.

// This suite mounts several trees into one document. Without cleanup the
// second assertion reads the first render's DOM.
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The session probe, routed with no object grants: the ordinary native seat
// this tab is drawn for. Unrouted it is refused, a refused session reads as a
// malformed one, and the tab draws the branch a denied grant produces — which
// a case wins alone and loses under load, on a different name each run.
//
// The stub's fallback answers an empty page, which is also what a narrowed
// read gets here: a kind dial that is a SERVER parameter makes the list its
// own request rather than the 360's seeded page.
beforeEach(() => {
  stubWithSession({}, {});
});

function withProviders(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const view: Person360 = {
  as_of: "2026-08-13T09:00:00Z",
  person: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
  sections_omitted: [],
  activities: {
    data: [
      activity({
        id: "a-1",
        kind: "email",
        subject: "Fleet renewal",
        direction: "inbound",
        occurred_at: "2026-08-11T12:00:00Z",
      }),
      activity({
        id: "a-2",
        kind: "meeting",
        subject: "Depot walkthrough",
        occurred_at: "2026-08-09T08:00:00Z",
      }),
    ],
    page: { has_more: false },
  },
  deal_roles: {
    data: [
      {
        relationship_id: "r-1",
        deal_id: "d-1",
        deal_title: "Fleet renewal 2026",
        deal_stage: "Proposal",
        role: "economic_buyer",
      },
    ],
    page: { has_more: false },
  },
  next_meeting: {
    activity_id: "a-9",
    starts_at: "2026-08-20T13:00:00Z",
    subject: "Contract review",
    participants: [{ person_id: "p-1", full_name: "Dana Buyer" }],
  },
};

// The same record read by someone whose grant reaches none of it: the sections
// are absent AND named, which is what separates "you may not see this" from
// "there is none".
const withheld: Person360 = {
  ...view,
  activities: undefined,
  deal_roles: undefined,
  next_meeting: undefined,
  sections_omitted: ["activities", "deal_roles", "next_meeting"],
};

describe("the timeline tab", () => {
  it("draws the exchanges the 360 carried", () => {
    withProviders(<PersonTimelineTab personId="p-1" view={view} />);
    expect(screen.getByText("Fleet renewal")).toBeTruthy();
  });

  it("opens on the whole chronology rather than on one cut of it", () => {
    // What was said and what changed are one order of events, and a reader who
    // wanted them together had to know a cut existed and choose it. The two
    // narrower cuts stay for a reader who wants only one of them.
    withProviders(<PersonTimelineTab personId="p-1" view={view} />);
    const pressed = (name: string) =>
      screen.getByRole("button", { name }).getAttribute("aria-pressed");
    expect(pressed("All")).toBe("true");
    expect(pressed("Activities")).toBe("false");
  });

  // The cuts stand in the panel's body, over the row that narrows whichever
  // cut is open. A pill row wraps to as many rows as the column needs, and the
  // head is one fixed band — so in the head it was the one card on the page
  // whose title sat at a different height from every other.
  it("keeps the cuts under the head, above the narrowing row", () => {
    const { container } = withProviders(
      <PersonTimelineTab personId="p-1" view={view} />,
    );
    const cuts = screen.getByRole("button", { name: "All" });
    expect(cuts.closest(".panel-head")).toBeNull();
    const dials = container.querySelector(".panel-body .timeline-header");
    expect(dials).not.toBeNull();
    expect(dials?.contains(cuts)).toBe(true);
    // The narrowing row follows the cuts inside the same block, so the two
    // read as one set of dials rather than as a control that wrapped.
    expect(dials?.querySelector(".timeline-filters")).not.toBeNull();
  });

  // Changes is the one cut with no exchanges to narrow, so the filter row
  // stands down — and the cuts themselves must not go with it, or a reader who
  // picked Changes has no way back to the rest of the chronology.
  it("keeps the cuts when the narrowing row stands down", async () => {
    const user = userEvent.setup();
    withProviders(<PersonTimelineTab personId="p-1" view={view} />);

    await user.click(screen.getByRole("button", { name: "Changes" }));

    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("false");
    expect(screen.queryByLabelText("Activity kind")).toBeNull();
  });

  // The record's chronology holds the exchanges and the record's own edits, and
  // between the whole and one kind sits the reading a reader most often comes
  // for: the conversations. It is the same chronicle cut, not a second
  // rendering — the account page has offered it since it shipped, and a contact
  // is where a conversation actually happens.
  it("offers the conversations cut and draws only what somebody can answer", async () => {
    const user = userEvent.setup();
    withProviders(<PersonTimelineTab personId="p-1" view={view} />);

    await user.click(screen.getByRole("button", { name: "Conversations" }));

    // Waited on the settled cut rather than asserted straight after the press:
    // the mail is on screen under BOTH cuts, so what says the cut took is the
    // meeting leaving.
    await waitFor(() =>
      // A meeting is an event, not an exchange. It stays on the cuts that are
      // about the record's whole chronology.
      expect(screen.queryByText("Depot walkthrough")).toBeNull(),
    );
    expect(screen.getByText("Fleet renewal")).toBeTruthy();
  });

  // The dial narrows the cut it stands under. Offering Meetings inside a cut
  // that keeps only mail and messages names a value whose only possible answer
  // is an empty list — on a record whose mail is right there under the pill.
  it("offers the conversations cut only the kinds it can draw", async () => {
    const user = userEvent.setup();
    withProviders(<PersonTimelineTab personId="p-1" view={view} />);

    await user.click(screen.getByRole("button", { name: "Conversations" }));
    await user.click(screen.getByLabelText("Activity kind"));

    const listbox = await screen.findByRole("listbox");
    const offered = within(listbox)
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(offered).toEqual(["All kinds", "Email", "Messages"]);
  });

  // The kind is a SERVER parameter and the cut is a client-side narrowing, so
  // the two can contradict each other. Opening the cut narrows what the reader
  // asked for rather than leaving the server answering about meetings while
  // every row it returns is thrown away.
  it("drops a kind the conversations cut cannot draw as it opens", async () => {
    const user = userEvent.setup();
    withProviders(<PersonTimelineTab personId="p-1" view={view} />);

    await pickOption(user, screen.getByLabelText("Activity kind"), "Meetings");
    await user.click(screen.getByRole("button", { name: "Conversations" }));

    await waitFor(() =>
      expect(screen.getByLabelText("Activity kind").textContent).toContain(
        "All kinds",
      ),
    );
  });

  it("says the section is withheld rather than drawing it empty", () => {
    withProviders(<PersonTimelineTab personId="p-1" view={withheld} />);
    expect(screen.queryByText(/Nothing has been logged/)).toBeNull();
    expect(
      screen.getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();
  });

  // The same obligation on the new cut, which reads that same withheld
  // section: "no conversations with them yet" about a section the reader's
  // grant does not reach is the page telling them a relationship never
  // happened.
  it("says so on the conversations cut too, rather than reporting none", async () => {
    const user = userEvent.setup();
    withProviders(<PersonTimelineTab personId="p-1" view={withheld} />);

    await user.click(screen.getByRole("button", { name: "Conversations" }));

    expect(screen.queryByText(/No conversations with them yet/)).toBeNull();
    expect(
      screen.getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();
  });
});

describe("the deals tab", () => {
  it("names every deal the person is recorded on, with their seat", () => {
    withProviders(<PersonDealsTab view={view} />);
    expect(screen.getByText("Fleet renewal 2026")).toBeTruthy();
    expect(screen.getByText("Proposal")).toBeTruthy();
    expect(screen.getByText("Economic buyer")).toBeTruthy();
  });

  it("does not report an absent grant as an absence of deals", () => {
    withProviders(<PersonDealsTab view={withheld} />);
    expect(screen.queryByText(/not recorded on any deal/)).toBeNull();
  });

  // THE TAB'S OWN VERB. A reader asking which deals this contact is on is the
  // reader who wants to put them on one, and before this the only path was the
  // relationships tab — a different question, reached from a different place.
  it("offers the verb that seats this contact on a deal", async () => {
    const searched: string[] = [];
    const page = (rows: { id: string; name: string }[]) =>
      new Response(
        JSON.stringify({
          data: rows,
          page: { has_more: false, next_cursor: null },
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    stubWithSession(
      {
        "GET /deals": () => {
          searched.push("deals");
          // A name NOT already on the tab. "Fleet renewal 2026" is the row the
          // page draws, so waiting for it would pass on the rendering the
          // picker had nothing to do with.
          return page([{ id: "d-9", name: "Depot expansion 2027" }]);
        },
        "GET /companies": () => {
          searched.push("companies");
          return page([]);
        },
      },
      { relationship: ["create"] },
    );
    withProviders(<PersonDealsTab view={view} />);

    const add = await screen.findByRole("button", { name: "Add to a deal" });
    await userEvent.click(add);

    expect(screen.getByRole("heading", { name: "Add to a deal" })).toBeTruthy();

    // NARROWED TO THE DEAL EDGE, asserted on what the picker SEARCHES.
    //
    // Two weaker assertions were tried and both passed with the narrowing
    // removed: the kind selector is hidden either way, and no option label is
    // rendered when it is hidden. What the narrowing actually decides is which
    // endpoint the dialog picks first — unnarrowed, a person scope leads with
    // `employment` and the picker searches COMPANIES, so a reader typing a deal
    // name would find nothing and never learn why.
    await userEvent.type(
      screen.getByRole("searchbox", { name: /search/i }),
      "Depot",
    );
    await waitFor(
      () => {
        expect(screen.getByText("Depot expansion 2027")).toBeTruthy();
      },
      { timeout: 2000 },
    );
    expect(searched).toContain("deals");
    expect(searched).not.toContain("companies");
  });

  // Withheld rather than disabled: a grant the role does not hold is not a fact
  // about this contact, so there is nothing for a disabled button to explain.
  it("withholds the verb from a reader who may not write an edge", async () => {
    stubWithSession({}, {});
    withProviders(<PersonDealsTab view={view} />);

    // Awaited through the rows, so the absence is read AFTER the grant probe
    // has answered rather than before it has run.
    await screen.findByText("Fleet renewal 2026");
    expect(screen.queryByRole("button", { name: "Add to a deal" })).toBeNull();
  });
});

describe("the meetings tab", () => {
  it("puts the booked meeting above the ones already held", () => {
    withProviders(<PersonMeetingsTab view={view} />);
    expect(screen.getByText("Contract review")).toBeTruthy();
    expect(screen.getByText("Depot walkthrough")).toBeTruthy();
  });

  it("draws only the meetings, never the whole chronology", () => {
    withProviders(<PersonMeetingsTab view={view} />);
    // The email in the same activities page belongs to the Activity tab. A
    // filter that let it through here would make this tab a second, worse
    // spelling of that one.
    expect(screen.queryByText("Fleet renewal")).toBeNull();
  });

  it("offers a brief for the booked meeting and for one already held", async () => {
    // The backend assembles a brief for ANY meeting activity. Reaching it only
    // through the next meeting's prep moment left every other meeting on the
    // record with a brief nothing could ask for.
    const briefed: string[] = [];
    withProviders(
      <PersonMeetingsTab
        view={view}
        onBriefMeeting={(id) => briefed.push(id)}
      />,
    );
    const actions = screen.getAllByRole("button", { name: "Brief me" });
    expect(actions.length).toBe(2);
    await userEvent.setup().click(actions[0]);
    expect(briefed.length).toBe(1);
  });

  it("offers no brief for a meeting the reader may find but not read", () => {
    // The timeline carries discoverable-but-withheld rows on purpose, so the
    // reader knows a conversation happened. The brief endpoint applies the
    // stricter content gate, so a verb here would promise what their own grant
    // refuses — and answer 404 when they took it up.
    const withWithheld: Person360 = {
      ...view,
      activities: {
        data: [
          activity({
            id: "a-secret",
            kind: "meeting",
            subject: "Board session",
            occurred_at: "2026-08-10T08:00:00Z",
            content_state: "withheld",
          }),
        ],
        page: { has_more: false },
      },
      next_meeting: undefined,
    };
    withProviders(
      <PersonMeetingsTab view={withWithheld} onBriefMeeting={() => {}} />,
    );
    // The row is DRAWN — the reader learns a meeting happened — and carries no
    // verb. Asserting the subject would be wrong: a withheld row redacts it,
    // which is the whole point of the state.
    expect(screen.queryByText(/Nothing logged/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Brief me" })).toBeNull();
  });

  it("offers no brief when the surface cannot open one", () => {
    // Without the callback the verb would be a button that does nothing, which
    // teaches a reader the feature is broken rather than absent.
    withProviders(<PersonMeetingsTab view={view} />);
    expect(screen.queryByRole("button", { name: "Brief me" })).toBeNull();
  });

  it("names the meeting the reader picked, not the soonest one", async () => {
    // The defect this replaces: the drawer always read next_meeting, so a
    // brief opened from a past meeting described a different room.
    const briefed: string[] = [];
    withProviders(
      <PersonMeetingsTab
        view={view}
        onBriefMeeting={(id) => briefed.push(id)}
      />,
    );
    const actions = screen.getAllByRole("button", { name: "Brief me" });
    // The booked meeting leads the tab; the held one follows it.
    await userEvent.setup().click(actions[1]);
    expect(briefed).toEqual(["a-2"]);
  });
});
