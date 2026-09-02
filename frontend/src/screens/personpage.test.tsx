/** @vitest-environment jsdom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { PersonPageV2 } from "./personpage";
import { PERSON_TABS } from "./persontab";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Every tab of the contact record was a placeholder panel once, and the way
// that regresses is silent: a tab whose content stops rendering looks exactly
// like a tab with nothing on it. These mount the REAL page — the tab bar, the
// panel switch and the routing between them — because the bug this file exists
// to catch lived in the seam between them, not inside any one tab.

type Person360 = components["schemas"]["Person360"];
type PersonConsentGuardEntry = components["schemas"]["PersonConsentGuardEntry"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

const view: Person360 = {
  as_of: "2026-08-13T09:00:00Z",
  // With an address on file, so the header's lead verb is the Email one. The
  // verb names the transport the composer would pick, so a contact with no way
  // to be reached carries a different button — which is its own suite below.
  person: {
    id: "p-1",
    full_name: "Dana Buyer",
    emails: [
      {
        id: "pe-1",
        person_id: "p-1",
        email: "dana@brandt.example",
        email_type: "work",
        is_primary: true,
        position: 0,
        ...CAPTURED,
      },
    ],
    ...CAPTURED,
  },
  sections_omitted: [],
  activities: {
    data: [
      {
        id: "a-1",
        kind: "email",
        subject: "Fleet renewal",
        occurred_at: "2026-08-11T12:00:00Z",
        is_done: false,
        ...CAPTURED,
      },
    ],
    page: { has_more: false },
  },
  deal_roles: { data: [], page: { has_more: false } },
  profile_fields: [],
};

// The gone-quiet rung, carrying the reason the server put in its destination.
// The label promises a follow-up; the prefill is the only place the reason it
// fired for is written down.
const quietMoment: Person360["moment"] = {
  claim_key: "gone_quiet:p-1",
  evidence_fingerprint: "fp-1",
  rule: "gone_quiet",
  headline: "Dana has gone quiet",
  why_now: "Six weeks since her last reply",
  confidence: "observed_fact",
  evidence: [{ type: "activity", label: "Fleet renewal", id: "a-1" }],
  recommended_action: {
    kind: "draft_reply",
    label: "Draft a follow-up",
    state: "available",
    destination: { surface: "composer", prefill: { intent: "follow_up" } },
  },
};

// A guard that permits mail, so the header's own Email verb is pressable: the
// hero button is disabled until some purpose says yes, and the empty-composer
// case can only be read through a button a reader can press.
const mailAllowed: PersonConsentGuardEntry = {
  purpose_key: "business_correspondence",
  purpose_class: "business_correspondence",
  channel: "email",
  verdict: "allowed",
  reason: "she wrote to you on 11 August",
};

function mount(
  tab: (typeof PERSON_TABS)[number],
  page: Person360 = view,
  guardEntries: readonly PersonConsentGuardEntry[] = [],
  // Routes a test adds for the write it makes; the reads every mount needs
  // stay here.
  extraRoutes: RouteMap = {},
  // What the caller may do. Logging needs `activity.create`, which the store
  // behind the form requires, so a spec that logs must hold it here or it
  // passes under an authorization production refuses.
  allow: Parameters<typeof meRoute>[0] = {
    person: ["read", "update"],
    activity: ["create"],
  },
) {
  installFetchStub({
    ...extraRoutes,
    "GET /me": meRoute(allow, { seat: "full" }),
    "GET /people/p-1/360": () => jsonResponse(page),
    "GET /people/p-1/brief": () =>
      jsonResponse({ person_id: "p-1", sentences: [], generated_by: "rules" }),
    "GET /people/p-1/consent/guard": () =>
      jsonResponse({ person_id: "p-1", entries: guardEntries }),
    // Which messaging providers exist is a deployment fact, so a channel has
    // no name until the directory supplies one.
    "GET /channel-providers": () =>
      jsonResponse({
        data: [
          {
            provider: "zalo_oa",
            label: "Zalo OA",
            credential_model: "workspace_bot",
            supplies_transport: true,
          },
        ],
      }),
  });
  render(
    <StoryProviders>
      <PersonPageV2 id="p-1" tab={tab} />
    </StoryProviders>,
  );
}

// The record's header, which is where the page's verbs live. Found through the
// heading it names rather than by class, because the header is a landmark for
// the record and the rail offers verbs of its own that must not be mistaken
// for it.
async function recordHeader(): Promise<HTMLElement> {
  const name = await screen.findByRole("heading", {
    level: 1,
    name: "Dana Buyer",
  });
  const header = name.closest("header");
  if (!(header instanceof HTMLElement)) {
    throw new Error("the record's header is not around its own heading");
  }
  return header;
}

// The page's ONE primary action, once the reads it depends on have arrived: the
// consent guard decides whether it may be pressed and the transport directory
// names a channel, so the first paint is not the answer. Waiting on the LABEL
// is what makes that wait a condition rather than a duration.
//
// Asserting the primary variant is half the point of every case below: what is
// under test is what the page's ONE green verb claims, not that some button
// somewhere carries the word.
async function leadVerb(label: string): Promise<HTMLButtonElement> {
  const header = await recordHeader();
  const lead = await within(header).findByRole("button", { name: label });
  if (!(lead instanceof HTMLButtonElement)) {
    throw new Error(`the header's "${label}" verb is not a button`);
  }
  if (!lead.classList.contains("btn-primary")) {
    throw new Error(`"${label}" is not the page's primary action`);
  }
  return lead;
}

afterEach(() => {
  cleanup();
});

describe("the contact record's tabs", () => {
  it.each(PERSON_TABS.filter((tab) => tab !== "overview"))(
    "draws real content on the %s tab rather than an empty column",
    async (tab) => {
      mount(tab);
      // Whatever the tab's own state turns out to be, it must render a panel
      // of its own: an empty main column is the failure this pins.
      const panels = await screen.findAllByRole("heading", { level: 2 });
      expect(panels.length).toBeGreaterThan(0);
    },
  );

  it("sends the header's Call verb to the tab that holds the conversation", async () => {
    // The tab ids are URL segments and `navigate` takes them as a bare string,
    // so a rename can leave a verb pointing at an id nothing serves — the
    // router then falls back to Overview and says nothing about it.
    const user = userEvent.setup();
    mount("overview");
    // The header's verb, not the rail's: the rail offers its own Call, and
    // the two land in different places on purpose.
    const header = await recordHeader();
    await user.click(within(header).getByRole("button", { name: "Call" }));
    expect(window.location.hash).toBe("#/contacts/p-1/timeline");
  });
});

describe("the overview tab's added capabilities", () => {
  it("lets a reader confirm or correct a field Margince read off a signature", async () => {
    const withEnrichment: Person360 = {
      ...view,
      profile_fields: [
        {
          field: "title",
          value: "Head of Procurement",
          evidence_snippet: "Head of Procurement, Brandt Automotive GmbH",
          source: "capture_enrich",
          captured_by: "agent:enrich",
          captured_at: "2026-08-01T08:00:00Z",
        },
      ],
    };
    mount("overview", withEnrichment);
    expect(await screen.findByText("What Margince read")).toBeTruthy();
    expect(screen.getByText("Head of Procurement")).toBeTruthy();
    // The heading and the value alone don't prove the page reaches the
    // control the ticket was about — the confirm/correct buttons themselves
    // have to be there too, or this passes on a row that reads but does not
    // write.
    // Both buttons are gated on the caller's own grant, off a SEPARATE query
    // (/me) than the field's own read — findByRole waits it out rather than
    // catching the render before that query has settled.
    expect(
      await screen.findByRole("button", { name: "That is right" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Correct" })).toBeTruthy();
  });
});

describe("a moment action that opens the composer", () => {
  // The steering field's own value, read off the element rather than through a
  // matcher: this file carries no jest-dom, and narrowing beats asserting.
  async function intentValue(): Promise<string> {
    const field = await screen.findByRole("textbox", {
      name: "What should it be about?",
    });
    if (!(field instanceof HTMLInputElement)) {
      throw new Error("the composer's steering field is not a text input");
    }
    return field.value;
  }

  it("opens it about the reason the rung fired for", async () => {
    // The rung computes WHY it fired and says so in its destination. A composer
    // that opened empty threw that away and left a labelled verb doing exactly
    // what the generic Write-an-email button does.
    const user = userEvent.setup();
    mount("overview", { ...view, moment: quietMoment }, [mailAllowed]);

    await user.click(
      await screen.findByRole("button", { name: "Draft a follow-up" }),
    );

    expect(await intentValue()).toBe("follow up — it has gone quiet");
  });

  it("opens the shared compose drawer for the generic Email verb, not the steering composer", async () => {
    // Mail as the only way in goes to the drawer every record shares — the
    // one that knows the record's conversations. The rung's labelled verb
    // above keeps the steering composer, because it arrives carrying an
    // intent the shared drawer has no field for.
    const user = userEvent.setup();
    mount("overview", { ...view, moment: quietMoment }, [mailAllowed]);

    const header = await recordHeader();
    await user.click(within(header).getByRole("button", { name: "Email" }));

    expect(
      await screen.findByRole("dialog", { name: /Send this email/ }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("textbox", { name: "What should it be about?" }),
    ).toBeNull();
  });
});

// The reachability the record carries, and the conversation a reply would
// continue. Both are needed before a channel is a way to reach anybody: the
// first says the identity is live, the second that there is something to answer.
const reachableOnZalo = [
  { provider: "zalo_oa", reachable: true, since: "2026-07-01T08:00:00Z" },
];

// `kind` is annotated rather than left to inference: the contract's activity
// kind is a closed union, and a bare object literal widens it to `string`, which
// the timeline's own type then refuses.
const chatMessage: components["schemas"]["Activity"] = {
  id: "a-chat",
  kind: "message",
  channel_provider: "zalo_oa",
  occurred_at: "2026-08-12T12:00:00Z",
  is_done: false,
  ...CAPTURED,
};

// A contact captured over a chat channel: no address, one conversation.
const chatOnly: Person360 = {
  ...view,
  person: { ...view.person, emails: [], reachability: reachableOnZalo },
  activities: { data: [chatMessage], page: { has_more: false } },
};

// Reachable both ways, which is the case where the composer must ASK.
const mailAndChat: Person360 = {
  ...view,
  person: { ...view.person, reachability: reachableOnZalo },
  activities: {
    data: [...(view.activities?.data ?? []), chatMessage],
    page: { has_more: false },
  },
};

// Nothing to write to: no address, and no channel conversation to answer. The
// mail thread on the record is not a way to reach them — an address is.
const unreachable: Person360 = {
  ...view,
  person: { ...view.person, emails: [] },
};

// A guard that refuses mail. Consent is decided per purpose, so this is one
// refused purpose and no allowed one — the state the hero button reads as "may
// not write".
const mailBlocked: PersonConsentGuardEntry = {
  purpose_key: "marketing",
  purpose_class: "marketing",
  channel: "email",
  verdict: "blocked",
  reason: "opt-out recorded 12 July",
};

describe("the page's one primary action", () => {
  // The two sentences a refusal can say. Held as constants because the point of
  // the last case is that these two are never the same sentence.
  const NO_TRANSPORT = "No address, and no conversation to reply to.";
  const CONSENT_REFUSED = "No purpose currently permits writing to them.";

  it("names mail when an address is the only way to reach them", async () => {
    mount("overview", view, [mailAllowed]);

    await waitFor(async () => {
      expect((await leadVerb("Email")).disabled).toBe(false);
    });
    const lead = await leadVerb("Email");
    expect(lead.querySelector(".lucide-mail")).toBeTruthy();
  });

  it("names the channel when a chat conversation is the only way", async () => {
    // The reported symptom: a green Email on a contact the CRM can only reach
    // on a chat channel. The provider is named by the directory, so this reads
    // as itself for a unit this build has never heard of too.
    mount("overview", chatOnly, [mailAllowed]);

    await waitFor(async () => {
      expect((await leadVerb("Message on Zalo OA")).disabled).toBe(false);
    });
    const lead = await leadVerb("Message on Zalo OA");
    expect(lead.querySelector(".lucide-message-square")).toBeTruthy();
    expect(lead.querySelector(".lucide-mail")).toBeNull();
  });

  it("promises neither transport when the composer will ask which", async () => {
    // Two ways to reach them means the drawer opens a picker, so a button
    // reading Email would have named one of several and then asked.
    const user = userEvent.setup();
    mount("overview", mailAndChat, [mailAllowed]);

    await waitFor(async () => {
      expect((await leadVerb("Write")).disabled).toBe(false);
    });
    const lead = await leadVerb("Write");
    expect(lead.querySelector(".lucide-pen-line")).toBeTruthy();
    expect(lead.querySelector(".lucide-mail")).toBeNull();
    expect(lead.querySelector(".lucide-message-square")).toBeNull();

    // And the picker it stayed neutral for is really there.
    await user.click(lead);
    expect(await screen.findByLabelText("How to send")).toBeTruthy();
  });

  it("refuses, and says why, when there is no way to write to them", async () => {
    // The case the consent verdict cannot catch: the guard is happily `allowed`
    // for a contact who has no address at all, so an enabled button would open
    // a composer that can only fail at the send.
    mount("overview", unreachable, [mailAllowed]);

    expect(await screen.findByText(NO_TRANSPORT)).toBeTruthy();
    const lead = await leadVerb("Write");
    expect(lead.disabled).toBe(true);
    expect(lead.getAttribute("aria-describedby")).toBeTruthy();
  });

  it("refuses under the consent verdict on its own reason", async () => {
    mount("overview", view, [mailBlocked]);

    expect(await screen.findByText(CONSENT_REFUSED)).toBeTruthy();
    const lead = await leadVerb("Email");
    expect(lead.disabled).toBe(true);
    // The verb still names the transport it would have opened: what changed is
    // whether it may be pressed, not what pressing it would do.
    expect(lead.querySelector(".lucide-mail")).toBeTruthy();
  });

  it("keeps the two refusals apart when both apply", async () => {
    // A rep told the wrong one goes looking in the wrong record. Reachability
    // is the sentence to show, because it is the half that no consent decision
    // can lift.
    mount("overview", unreachable, [mailBlocked]);

    expect(await screen.findByText(NO_TRANSPORT)).toBeTruthy();
    expect(screen.queryByText(CONSENT_REFUSED)).toBeNull();
  });
});

// The seam the meeting brief regressed at: the page decides WHICH meeting the
// drawer briefs. Mounting the tab alone proves only that a callback fires with
// an id — the page could still ignore it and ask for the next meeting, which
// is exactly the bug the drawer used to have. So this asserts the URL that
// actually goes out.
describe("which meeting the brief drawer asks about", () => {
  const heldMeeting = {
    id: "a-held",
    kind: "meeting" as const,
    subject: "Depot walkthrough",
    occurred_at: "2026-08-09T08:00:00Z",
    is_done: false,
    ...CAPTURED,
  };

  const withMeetings: Person360 = {
    ...view,
    activities: { data: [heldMeeting], page: { has_more: false } },
    next_meeting: {
      activity_id: "a-booked",
      starts_at: "2026-08-20T13:00:00Z",
      subject: "Contract review",
      participants: [{ person_id: "p-1", full_name: "Dana Buyer" }],
    },
  };

  // Which meeting the drawer asked the server about, recorded per test. The
  // routes are shared because four tests below all turn on the SAME question —
  // was the brief fetched, and for which meeting — and a second copy of them
  // would let one drift into stubbing a meeting the others do not have.
  function briefRoutes(asked: string[]): RouteMap {
    const brief = (id: string) => () => {
      asked.push(id);
      return jsonResponse({
        activity_id: id,
        generated_at: "2026-08-13T09:00:00Z",
        generated_by: "deterministic",
        sections: [],
      });
    };
    return {
      "GET /me": meRoute({ person: ["read", "update"], activity: ["read"] }),
      "GET /people/p-1/360": () => jsonResponse(withMeetings),
      "GET /people/p-1/brief": () =>
        jsonResponse({
          person_id: "p-1",
          sentences: [],
          generated_by: "rules",
        }),
      "GET /people/p-1/consent/guard": () =>
        jsonResponse({ person_id: "p-1", entries: [] }),
      "GET /channel-providers": () => jsonResponse({ data: [] }),
      "GET /activities/a-held/meeting-brief": brief("a-held"),
      "GET /activities/a-booked/meeting-brief": brief("a-booked"),
    };
  }

  it("requests the brief for the meeting the reader picked", async () => {
    const asked: string[] = [];
    installFetchStub(briefRoutes(asked));
    render(
      <StoryProviders>
        <PersonPageV2 id="p-1" tab="meetings" />
      </StoryProviders>,
    );

    const actions = await screen.findAllByRole("button", { name: "Brief me" });
    // The booked meeting leads the tab and the held one follows it, so the
    // second verb is the one that used to be unreachable.
    await userEvent.setup().click(actions[1]);

    await waitFor(() => expect(asked.length).toBe(1));
    expect(asked).toEqual(["a-held"]);
  });

  // The brief is the one drawer another SCREEN sends a reader to, so it is the
  // one with an address. Held in useState alone it could only ever be opened by
  // pressing a button on this page, which is not what a "prepare the meeting"
  // link on the worklist means.
  it("opens the brief the address names, with nothing pressed", async () => {
    const asked: string[] = [];
    installFetchStub(briefRoutes(asked));
    window.location.hash = "#/contacts/p-1/meetings?prep=a-held";
    render(
      <StoryProviders>
        <PersonPageV2 id="p-1" tab="meetings" />
      </StoryProviders>,
    );

    await waitFor(() => expect(asked).toEqual(["a-held"]));
  });

  // The reason it is derived from the address rather than seeded from it: a
  // drawer in useState stays open when the reader navigates back out of it,
  // because nothing tells it the address changed. Back is how a reader closes
  // a thing they arrived at by link, and it has to work.
  it("closes the brief when the address stops naming it", async () => {
    const asked: string[] = [];
    installFetchStub(briefRoutes(asked));
    window.location.hash = "#/contacts/p-1/meetings?prep=a-held";
    render(
      <StoryProviders>
        <PersonPageV2 id="p-1" tab="meetings" />
      </StoryProviders>,
    );
    await waitFor(() => expect(asked).toEqual(["a-held"]));

    window.location.hash = "#/contacts/p-1/meetings";
    window.dispatchEvent(new HashChangeEvent("hashchange"));

    // The BRIEF's dialog by its own label, not "a dialog": the page carries
    // other modals, and querying for any of them would pass on a page where
    // the brief never closed and something else happened to be shut.
    await waitFor(() =>
      expect(
        document.querySelector('[aria-labelledby="person-meeting-title"]'),
      ).toBeNull(),
    );
  });

  // A link somebody edited by hand, or one whose meeting has since been
  // deleted. The page renders; it does not throw and it does not open an empty
  // drawer that asks the server about nothing.
  it("renders normally when the address names no meeting", async () => {
    const asked: string[] = [];
    installFetchStub(briefRoutes(asked));
    window.location.hash = "#/contacts/p-1/meetings?prep=";
    render(
      <StoryProviders>
        <PersonPageV2 id="p-1" tab="meetings" />
      </StoryProviders>,
    );

    await screen.findAllByRole("button", { name: "Brief me" });
    expect(asked).toEqual([]);
  });
});

// The header's meta strip carries a link ICON beside the word LinkedIn, and for
// a while carried no link — a reader who saw the chain and clicked it got
// nothing. An affordance that looks like a link has to be one.
describe("the contact's LinkedIn on the header", () => {
  const withLinkedin: Person360 = {
    ...view,
    person: {
      ...view.person,
      social: { linkedin: "https://www.linkedin.com/in/dana-buyer" },
    },
  };

  it("opens the profile it names", async () => {
    mount("overview", withLinkedin);

    const header = await recordHeader();
    const link = within(header).getByRole("link", { name: "LinkedIn" });
    expect(link.getAttribute("href")).toBe(
      "https://www.linkedin.com/in/dana-buyer",
    );
    expect(link.getAttribute("target")).toBe("_blank");
    // Tokens rather than a substring: "notnoreferrer" contains "noreferrer"
    // and is not a relation any browser honours.
    const rel = (link.getAttribute("rel") ?? "").split(/\s+/);
    expect(rel).toContain("noopener");
    expect(rel).toContain("noreferrer");
  });

  it("says nothing at all when no profile is recorded", async () => {
    mount("overview");

    const header = await recordHeader();
    expect(within(header).queryByRole("link", { name: "LinkedIn" })).toBe(null);
    expect(within(header).queryByText("LinkedIn")).toBe(null);
  });

  it("falls back to plain text when the recorded value is not a web address", async () => {
    // `social` is an open map on the wire, so its values are whatever was
    // written there. A dead anchor is worse than no anchor.
    mount("overview", {
      ...view,
      person: { ...view.person, social: { linkedin: "in/dana-buyer" } },
    });

    const header = await recordHeader();
    expect(within(header).getByText("LinkedIn")).toBeTruthy();
    expect(within(header).queryByRole("link", { name: "LinkedIn" })).toBe(null);
  });

  it("refuses to put another host under the word LinkedIn", async () => {
    // The label is a fixed word, so it is a CLAIM about the destination — and
    // this value can be written by a crawl or a connector. An arbitrary host
    // under that word is a phishing link wearing the product's own chrome.
    mount("overview", {
      ...view,
      person: {
        ...view.person,
        social: { linkedin: "https://attacker.example/login" },
      },
    });

    const header = await recordHeader();
    // The fact is kept — this contact HAS something recorded — and the claim
    // that it is LinkedIn is what is withheld.
    expect(within(header).getByText("LinkedIn")).toBeTruthy();
    expect(within(header).queryByRole("link", { name: "LinkedIn" })).toBe(null);
  });

  it("accepts a regional LinkedIn subdomain", async () => {
    // The check is on the host, not on an exact string: de.linkedin.com is
    // LinkedIn, and refusing it would drop a working link for half of Europe.
    mount("overview", {
      ...view,
      person: {
        ...view.person,
        social: { linkedin: "https://de.linkedin.com/in/dana-buyer" },
      },
    });

    const header = await recordHeader();
    const link = within(header).getByRole("link", { name: "LinkedIn" });
    expect(link.getAttribute("href")).toBe(
      "https://de.linkedin.com/in/dana-buyer",
    );
  });
});

// The one thing a CRM must let a rep do on a person is write down what
// happened. Two ways in, one form: the header verb that is always there, and
// the moment card's action when its rung decides logging is the next step.
describe("logging an activity", () => {
  // The "nothing needed" rung, whose one action is to log an interaction.
  const quietDayMoment: Person360["moment"] = {
    claim_key: "moment:nothing_needed",
    evidence_fingerprint: "quiet",
    rule: "nothing_needed",
    headline: "Nothing needs you today",
    why_now:
      "No meeting is close, nothing is owed, and nobody is waiting on a reply.",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log an interaction",
      state: "available",
      destination: { surface: "activity_log" },
    },
  };

  it("opens the log form from the header, whatever the moment card offers", async () => {
    const user = userEvent.setup();
    mount("overview");

    // Disabled until the caller's grants have arrived, and the header is
    // rebuilt when they do — so the button is looked up fresh each time
    // rather than held from before the grant landed.
    const logButton = async () =>
      within(await recordHeader()).getByRole("button", {
        name: "Log activity",
      });
    await waitFor(async () =>
      expect(await logButton()).toHaveProperty("disabled", false),
    );
    await user.click(await logButton());

    expect(
      await screen.findByRole("dialog", { name: "Log activity" }),
    ).toBeTruthy();
  });

  it("keeps the verb on the page but refuses it without the create grant", async () => {
    // The store refuses a save without `activity.create`, so the button must
    // not open a form it cannot complete — and it stays visible, because a
    // reader who cannot tell "not mine to do" from "no such button" learns
    // nothing from an absence.
    mount("overview", view, [], {}, { person: ["read", "update"] });

    const header = await recordHeader();
    const button = within(header).getByRole("button", {
      name: /Log activity/,
    });
    expect(button).toHaveProperty("disabled", true);
    expect(
      within(header).getByText(
        "You do not have permission to log activities on this record.",
      ),
    ).toBeTruthy();
  });

  it("logs a meeting on this person from the moment card's action", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    mount("overview", { ...view, moment: quietDayMoment }, [], {
      "POST /activities": (body) => {
        posted.push(body);
        return jsonResponse({}, 201);
      },
    });

    await user.click(
      await screen.findByRole("button", { name: "Log an interaction" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await user.click(within(dialog).getByRole("combobox", { name: "Type" }));
    await user.click(await screen.findByRole("option", { name: "Meeting" }));
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "Coffee at the fair",
    );
    await user.click(within(dialog).getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "meeting",
      subject: "Coffee at the fair",
      meeting_status: "held",
      links: [{ entity_type: "person", entity_id: "p-1" }],
    });
  });
});
