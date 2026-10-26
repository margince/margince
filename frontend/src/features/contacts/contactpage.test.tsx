/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { CONTACT_TABS } from "../../lib/contacttab";
import {
  type Contact360,
  type ContactConsentGuardEntry,
  mount,
  quietMoment,
  recordHeader,
  view,
} from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactPageV2 } from "./contactpage";

afterEach(() => {
  cleanup();
});

// Every tab of the contact record was a placeholder panel once, and the way
// that regresses is silent: a tab whose content stops rendering looks exactly
// like a tab with nothing on it. These mount the REAL page — the tab bar, the
// panel switch and the routing between them — because the bug this file exists
// to catch lived in the seam between them, not inside any one tab.
//
// The shared fixture and the ordinary mount are contactpage.testkit.tsx's,
// also used by contactpage.editverbs.test.tsx — one fixture rather than a
// second copy the two suites could disagree about. Specs here that need a
// non-default mount (a route override outside `mount`'s own extraRoutes, a
// distinct grant) still reach the same seam directly.

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

// A guard that permits correspondence, so the rail's permissions panel says the
// ordinary thing. The hero Email verb does NOT read it: whether a message may
// go depends on the message, which only the engine in the composer can answer.
const mailAllowed: ContactConsentGuardEntry = {
  purpose_key: "business_correspondence",
  purpose_class: "business_correspondence",
  channel: "email",
  verdict: "allowed",
  reason: "they said you may write to them",
  reason_code: "said_we_may_write",
};

// The header's writing verb, once the reads it depends on have arrived: the
// consent guard decides whether it may be pressed and the transport directory
// names a channel, so the first paint is not the answer. Waiting on the LABEL
// is what makes that wait a condition rather than a duration.
async function leadVerb(label: string): Promise<HTMLButtonElement> {
  const header = await recordHeader();
  const lead = await within(header).findByRole("button", { name: label });
  if (!(lead instanceof HTMLButtonElement)) {
    throw new Error(`the header's "${label}" verb is not a button`);
  }
  return lead;
}

// The reachability the record carries, and the conversation a reply would
// continue. A channel reaches nobody without both. The first says the identity
// is live, the second that there is something to answer.
const reachableOnZalo = [
  { provider: "zalo_oa", reachable: true, since: "2026-07-01T08:00:00Z" },
];

// `kind` is annotated rather than left to inference. The contract's activity
// kind is a closed union, and a bare object literal widens it to `string`.
// The timeline's own type then refuses it.
const chatMessage: components["schemas"]["Activity"] = {
  occurred_precision: "instant",
  offers_email: false,
  id: "a-chat",
  kind: "message",
  channel_provider: "zalo_oa",
  occurred_at: "2026-08-12T12:00:00Z",
  is_done: false,
  ...CAPTURED,
};

// A contact captured over a chat channel: no address, one conversation.
const chatOnly: Contact360 = {
  ...view,
  contact: {
    ...view.contact,
    emails: [],
    primary_email: null,
    reachability: reachableOnZalo,
  },
  activities: { data: [chatMessage], page: { has_more: false } },
};

// Reachable both ways, which is the case where the composer must ASK.
const mailAndChat: Contact360 = {
  ...view,
  contact: { ...view.contact, reachability: reachableOnZalo },
  activities: {
    data: [...(view.activities?.data ?? []), chatMessage],
    page: { has_more: false },
  },
};

// Nothing to write to: no address, and no channel conversation to answer. The
// mail thread on the record is not a way to reach them; an address is.
const unreachable: Contact360 = {
  ...view,
  contact: { ...view.contact, emails: [], primary_email: null },
};

// A guard that refuses mail. Consent is decided per purpose, so this is one
// refused purpose and no allowed one: the state the hero button reads as "may
// not write".
const mailBlocked: ContactConsentGuardEntry = {
  purpose_key: "marketing",
  purpose_class: "marketing",
  channel: "email",
  verdict: "blocked",
  reason:
    "they objected to marketing, and an objection overrides every other basis",
  reason_code: "suppressed_objection",
};

describe("what the day's work says it could not read", () => {
  it("names the withheld moment, so a quiet day is not claimed off a source nobody read", async () => {
    // The moment is one of the two sources the panel is assembled from. With
    // it withheld and no open task, the rows are empty — and "nothing needs
    // you today" would be read off the one source that WAS read, as if it
    // were both.
    mount("overview", { ...view, sections_omitted: ["moments"] });
    expect(
      await screen.findByText(
        "Not included: Margince findings. You do not have access to them.",
      ),
    ).toBeTruthy();
  });
});

describe("the contact record's tabs", () => {
  it.each(CONTACT_TABS.filter((tab) => tab !== "overview"))(
    "draws real content on the %s tab rather than an empty column",
    async (tab) => {
      mount(tab);
      // Whatever the tab's own state turns out to be, it must render a panel
      // of its own: an empty main column is the failure this pins.
      const panels = await screen.findAllByRole("heading", { level: 2 });
      expect(panels.length).toBeGreaterThan(0);
    },
  );

  it("sends the header's Full history to the tab that holds the conversation", async () => {
    // The tab ids are URL segments and `navigate` takes them as a bare string,
    // so a rename can leave a verb pointing at an id nothing serves — the
    // router then falls back to Overview and says nothing about it.
    const user = userEvent.setup();
    mount("overview");
    const header = await recordHeader();
    const more = within(header).getByRole("button", {
      name: en["record.moreActions"],
    });
    await user.click(more);
    const menu = document.getElementById(
      more.getAttribute("aria-controls") ?? "",
    );
    if (menu === null) throw new Error("More opened no panel");
    await user.click(
      within(menu).getByRole("button", {
        name: en["record.fullHistory"],
      }),
    );
    expect(window.location.hash).toBe("#/contacts/p-1/timeline");
  });
});

describe("the Data & tools tab's added capabilities", () => {
  // On Data & tools, not the overview: an audit of what the enrichment pass
  // read is record-keeping, and the overview is what a rep opens before a
  // call.
  it("lets a reader confirm or correct a field Margince read off a signature", async () => {
    const withEnrichment: Contact360 = {
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
    mount("research", withEnrichment);
    expect(await screen.findByText("Captured details")).toBeTruthy();
    expect(screen.getByText("Head of Procurement")).toBeTruthy();
    // The heading and the value alone don't prove the page reaches the
    // control the ticket was about — the confirm/correct buttons themselves
    // have to be there too, or this passes on a row that reads but does not
    // write.
    // Both buttons are gated on the caller's own grant, off a SEPARATE query
    // (/me) than the field's own read — findByRole waits it out rather than
    // catching the render before that query has settled.
    expect(
      await screen.findByRole("button", { name: "Confirm value" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit" })).toBeTruthy();
  });
});

describe("a moment action that opens the composer", () => {
  // The steering field's own value, read off the element rather than through a
  // matcher: this file carries no jest-dom, and narrowing beats asserting.
  async function intentValue(): Promise<string> {
    await screen.findByRole("button", { name: /^Draft (reply|email)$/ });
    const field = await screen.findByRole("textbox", {
      name: /Purpose of the email|Purpose of the reply/,
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

    expect(await intentValue()).toBe("follow up after a quiet period");
  });

  it("opens the same drawer for the generic verb, carrying no reason", async () => {
    // ONE composer, two doors. The rung's labelled verb arrives carrying a
    // reason and the header's generic one does not, and that is the whole of
    // the difference: the surface, its thread pane and its conversations are
    // the same either way. Two drawers is what this replaced — the half of the
    // traffic that went to the older one got no thread beside the reply at all.
    const user = userEvent.setup();
    mount("overview", { ...view, moment: quietMoment }, [mailAllowed]);

    const header = await recordHeader();
    await user.click(within(header).getByRole("button", { name: "Email" }));

    expect(
      await screen.findByRole("dialog", { name: /New email/ }),
    ).toBeTruthy();
    // The steer field is here, and EMPTY: pressing the generic verb must not
    // inherit the reason the last rung left behind.
    expect(await intentValue()).toBe("");
  });

  it("opens the same drawer from the address in the details, never a mail client", async () => {
    // A third door, the same room. The address is a button into this page's
    // own composer: a `mailto:` would have handed the address to the reader's
    // mail client and the product would never have seen the message, and the
    // shell's composer beside this page's would be the two drawers the page
    // replaced.
    const user = userEvent.setup();
    mount("overview", { ...view, moment: quietMoment }, [mailAllowed]);

    const details = await recordDetails();
    const address = await within(details).findByRole("button", {
      name: "dana@brandt.example",
    });
    expect(address.hasAttribute("href")).toBe(false);
    await user.click(address);

    expect(
      await screen.findByRole("dialog", { name: /New email/ }),
    ).toBeTruthy();
    expect(screen.getAllByRole("dialog").length).toBe(1);
  });

  it("hands the address to the reader's own client when they have no mailbox to send from", async () => {
    // The composer sends from the reader's own connected mailbox. Without one
    // it could only refuse, so the address is a `mailto:` — the page's own
    // drawer is not offered for it either.
    mount("overview", { ...view, moment: quietMoment }, [mailAllowed], {
      "GET /connectors": () => jsonResponse({ data: [] }),
    });

    const details = await recordDetails();
    const address = await within(details).findByRole("link", {
      name: "dana@brandt.example",
    });
    expect(address.getAttribute("href")).toBe("mailto:dana@brandt.example");
    expect(
      within(details).queryByRole("button", { name: "dana@brandt.example" }),
    ).toBeNull();
  });
});

describe("the header's writing verb", () => {
  // The ONE sentence this verb's refusal can say. There is no consent sentence
  // to hold beside it, and that is the claim rather than an omission: the
  // composer asks about consent, because consent is answered per PURPOSE and
  // the purpose is a fact about the message.
  const NO_TRANSPORT = "No address and no thread to reply to.";

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
    expect(lead.querySelector(".lucide-send")).toBeTruthy();
    expect(lead.querySelector(".lucide-mail")).toBeNull();
    expect(lead.querySelector(".lucide-message-square")).toBeNull();

    // And the picker it stayed neutral for is really there.
    await user.click(lead);
    expect(await screen.findByLabelText(en["compose.transport"])).toBeTruthy();
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

  // WRITING IS ALWAYS AVAILABLE. A contact who stopped the newsletter can still
  // be sent their invoice, and which purpose applies is a question about the
  // MESSAGE — which this page does not have. Disabling the verb answered it
  // anyway, and answered it wrong: a rep with a lawful service message to send
  // was told the product would not let them write at all, with no way to see
  // that only marketing was refused.
  it("opens the composer even where no purpose currently permits writing", async () => {
    mount("overview", view, [mailBlocked]);

    const lead = await leadVerb("Email");
    await waitFor(() => {
      expect(lead.disabled).toBe(false);
    });
    // NO REFUSAL AT ALL, asserted on the control rather than by looking for a
    // sentence. The sentence this used to look for was withdrawn from the copy
    // catalogs with the behaviour, so `queryByText` for it could never find
    // anything and the case passed whatever the button did. What cannot go
    // vacuous is the button's own state: a refused verb is disabled and points
    // at its reason, and this one does neither.
    expect(lead.getAttribute("aria-describedby")).toBeNull();
    // The verb still names the transport it opens.
    expect(lead.querySelector(".lucide-mail")).toBeTruthy();
  });

  it("still refuses when there is nowhere to write to", async () => {
    // Reachability is the half no consent decision can lift: with no transport
    // the composer has nothing to send on, whatever the verdict says.
    mount("overview", unreachable, [mailBlocked]);

    expect(await screen.findByText(NO_TRANSPORT)).toBeTruthy();
    const lead = await leadVerb("Write");
    expect(lead.disabled).toBe(true);
  });
});

// The page decides WHICH meeting the drawer briefs. The tab mounted alone proves
// only that a callback fires; the page could still ask for the next meeting, the
// drawer's old bug. So this asserts the URL that actually goes out.
describe("which meeting the brief drawer asks about", () => {
  const heldMeeting = {
    offers_email: false,
    id: "a-held",
    kind: "meeting" as const,
    subject: "Depot walkthrough",
    occurred_at: "2026-08-09T08:00:00Z",
    occurred_precision: "instant" as const,
    is_done: false,
    ...CAPTURED,
  };

  const withMeetings: Contact360 = {
    ...view,
    activities: { data: [heldMeeting], page: { has_more: false } },
    next_meeting: {
      activity_id: "a-booked",
      starts_at: "2026-08-20T13:00:00Z",
      occurred_precision: "instant",
      subject: "Contract review",
      participants: [{ contact_id: "p-1", full_name: "Dana Buyer" }],
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
      "GET /me": meRoute({ contact: ["read", "update"], activity: ["read"] }),
      "GET /contacts/p-1/360": () => jsonResponse(withMeetings),
      "GET /contacts/p-1/brief": () =>
        jsonResponse({
          contact_id: "p-1",
          sentences: [],
          generated_by: "rules",
        }),
      "GET /contacts/p-1/consent/guard": () =>
        jsonResponse({ contact_id: "p-1", entries: [] }),
      "GET /channel-providers": () => jsonResponse({ data: [] }),
      "GET /activities": () => jsonResponse(withMeetings.activities),
      "GET /activities/a-held/meeting-brief": brief("a-held"),
      "GET /activities/a-booked/meeting-brief": brief("a-booked"),
    };
  }

  it("requests the brief for the meeting the reader picked", async () => {
    const asked: string[] = [];
    installFetchStub(briefRoutes(asked));
    render(
      <StoryProviders>
        <ContactPageV2 id="p-1" tab="meetings" />
      </StoryProviders>,
    );

    await screen.findByText("Depot walkthrough");
    const actions = screen.getAllByRole("button", {
      name: en["contact.meeting.brief"],
    });
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
        <ContactPageV2 id="p-1" tab="meetings" />
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
        <ContactPageV2 id="p-1" tab="meetings" />
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
        document.querySelector('[aria-labelledby="contact-meeting-title"]'),
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
        <ContactPageV2 id="p-1" tab="meetings" />
      </StoryProviders>,
    );

    await screen.findAllByRole("button", { name: en["contact.meeting.brief"] });
    expect(asked).toEqual([]);
  });
});

// The header's meta strip carries a link ICON beside the word LinkedIn, and for
// a while carried no link — a reader who saw the chain and clicked it got
// nothing. An affordance that looks like a link has to be one.
// The profile is a detail, so it is drawn in the aside, as a link only when
// the address really is LinkedIn's.
describe("the contact's LinkedIn in the details", () => {
  const withLinkedin: Contact360 = {
    ...view,
    contact: {
      ...view.contact,
      social: { linkedin: "https://www.linkedin.com/in/dana-buyer" },
    },
  };
  // The left column draws the profile's path as the link's words.
  const openProfile = "in/dana-buyer";

  it("opens the profile it names", async () => {
    mount("overview", withLinkedin);

    const link = within(await recordDetails()).getByRole("link", {
      name: openProfile,
    });
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

  it("says nothing in the head about a profile", async () => {
    mount("overview", withLinkedin);

    const header = await recordHeader();
    expect(within(header).queryByRole("link", { name: openProfile })).toBe(
      null,
    );
  });

  it("falls back to plain text when the recorded value is not a web address", async () => {
    // `social` is an open map on the wire, so its values are whatever was
    // written there. A dead anchor is worse than no anchor.
    mount("overview", {
      ...view,
      contact: { ...view.contact, social: { linkedin: "in/dana-buyer" } },
    });

    const aside = await recordDetails();
    expect(within(aside).getByText("in/dana-buyer")).toBeTruthy();
    expect(within(aside).queryByRole("link", { name: openProfile })).toBe(null);
  });

  it("refuses to put another host under the word LinkedIn", async () => {
    // The row's label is a fixed word, so it is a claim about the destination.
    // A crawl or a connector can write this value.
    mount("overview", {
      ...view,
      contact: {
        ...view.contact,
        social: { linkedin: "https://attacker.example/login" },
      },
    });

    const aside = await recordDetails();
    const row = within(aside)
      .getByText("https://attacker.example/login")
      .closest(".factlist-row");
    expect(row?.querySelector("a")).toBeNull();
  });

  it("accepts a regional LinkedIn subdomain", async () => {
    // The check is on the host, not on an exact string: de.linkedin.com is
    // LinkedIn, and refusing it would drop a working link for half of Europe.
    mount("overview", {
      ...view,
      contact: {
        ...view.contact,
        social: { linkedin: "https://de.linkedin.com/in/dana-buyer" },
      },
    });

    const link = within(await recordDetails()).getByRole("link", {
      name: openProfile,
    });
    expect(link.getAttribute("href")).toBe(
      "https://de.linkedin.com/in/dana-buyer",
    );
  });
});

// The details beside the feed, once the head has drawn.
async function recordDetails(): Promise<HTMLElement> {
  await recordHeader();
  const aside = document.querySelector<HTMLElement>(".record-aside");
  if (!aside) {
    throw new Error("the record drew no details beside the feed");
  }
  return aside;
}

// The composer another screen sends a reader to.
//
// The worklist's `draft_reply` move refused to promise a draft while the click
// only navigated. `?compose=reply` is the route that makes the promise true, so
// these are about it opening, saying what it was asked for, and closing on Back
// the way the brief drawer does.
describe("ContactPageV2 — the addressed composer", () => {
  afterEach(() => {
    window.location.hash = "";
  });

  it("opens the composer the address asks for, with nothing pressed", async () => {
    window.location.hash = "#/contacts/p-1?compose=reply";
    mount("overview");

    // The INTENT is what makes this different from the generic "Write an
    // email": the draft opens knowing what it is for. It is the field's VALUE
    // rather than text on the page — the composer hands it to a model.
    const intent = await screen.findByLabelText(
      /Purpose of the email|Purpose of the reply/,
    );
    expect((intent as HTMLInputElement).value).toBe(
      en["contact.composer.intentReply"],
    );
  });

  // An address is something a contact can type. A stray `?compose=x` silently
  // opening an empty draft is worse than a link that does nothing.
  it("opens nothing for an intent it does not know", async () => {
    window.location.hash = "#/contacts/p-1?compose=sell-them-something";
    mount("overview");

    await screen.findByRole("heading", { name: view.contact.full_name });
    expect(
      screen.queryByLabelText(/Purpose of the email|Purpose of the reply/),
    ).toBeNull();
  });

  // Derived from the address rather than seeded from it — the same reason the
  // brief drawer is. Back is how a reader closes a thing they arrived at by
  // link, and it has to work.
  it("closes when the address stops asking for it", async () => {
    window.location.hash = "#/contacts/p-1?compose=reply";
    mount("overview");
    await screen.findByLabelText(/Purpose of the email|Purpose of the reply/);

    window.location.hash = "#/contacts/p-1";
    window.dispatchEvent(new HashChangeEvent("hashchange"));

    await waitFor(() =>
      expect(
        screen.queryByLabelText(/Purpose of the email|Purpose of the reply/),
      ).toBeNull(),
    );
  });
});

// `writable` is the server's per-row answer, and every write on this page that
// changes the RECORD reads it: a rep holding contact.update on the OBJECT was
// offered Share and the upload on a colleague's contact and refused on submit.
describe("a live contact that is not the viewer's to change", () => {
  const notMine: Contact360 = {
    ...view,
    contact: { ...view.contact, owner_id: "u-other", writable: false },
  };
  const sentence =
    "You cannot edit this contact. Ask the owner to share it, or an administrator for edit rights.";

  it("says so once, and refuses Share from that sentence", async () => {
    const user = userEvent.setup();
    mount("overview", notMine);

    expect(await screen.findByText(sentence)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "More actions" }));
    const share = await screen.findByTestId("share-record");
    expect(share.hasAttribute("disabled")).toBe(true);
    expect(
      document.getElementById(share.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe(sentence);
  });

  it("refuses the upload on the Documents tab from the same sentence", async () => {
    mount("documents", notMine);

    const add = await screen.findByRole("button", { name: "Add document" });
    expect(add.hasAttribute("disabled")).toBe(true);
    expect(
      document.getElementById(add.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe(sentence);
  });
});

// The Research tab's consent panel says which purpose each verdict is for.
//
// The guard answers one verdict per purpose. The rail once took the first email
// entry and printed a bare "Allowed" against Email. A rep read a permission
// with no scope on it. The composer then refused the send, correctly, because
// the grant was for correspondence and the message was something else. Two
// true statements and no way to reconcile them.
describe("the consent panel names the purpose behind each answer", () => {
  const correspondence: ContactConsentGuardEntry = {
    purpose_key: "business_correspondence",
    purpose_label: "Business correspondence",
    purpose_class: "business_correspondence",
    channel: "email",
    verdict: "allowed",
    reason: "they said you may write to them",
    reason_code: "said_we_may_write",
  };
  const newsletter: ContactConsentGuardEntry = {
    purpose_key: "newsletter",
    purpose_label: "Newsletter",
    purpose_class: "marketing",
    channel: "email",
    verdict: "unknown",
    reason: "no consent recorded",
    reason_code: "no_consent",
  };

  it("answers per purpose rather than once for the whole transport", async () => {
    mount("research", view, [newsletter, correspondence]);

    // Both named, whichever order the server listed them in — the old reader
    // took entries[0] and would have painted Email with the newsletter's
    // unknown here.
    expect(await screen.findByText("Business correspondence")).not.toBeNull();
    expect(screen.getByText("Newsletter")).not.toBeNull();
  });

  it("does not report a marketing purpose's verdict against correspondence", async () => {
    mount("research", view, [newsletter, correspondence]);

    await screen.findByText("Business correspondence");
    // The correspondence row is the allowed one; the newsletter row is not.
    const rows = screen.getAllByText(/Allowed|Unknown/);
    expect(rows.length).toBeGreaterThan(1);
  });

  // A REFUSAL A REP CANNOT EXPLAIN IS NOT USABLE. The rail showed the reason
  // for correspondence alone, so a subject who asked us to stop marketing got a
  // blocked row with nothing under it saying why — the one fact a rep needs
  // before they pick up the phone about it.
  it("says why a marketing purpose is blocked, not only a correspondence one", async () => {
    const objected: ContactConsentGuardEntry = {
      purpose_key: "newsletter",
      purpose_label: "Newsletter",
      purpose_class: "marketing",
      channel: "email",
      verdict: "blocked",
      reason: "they asked you to stop, and a rep recorded it",
      reason_code: "suppressed_subject_request",
    };
    mount("research", view, [objected, correspondence]);

    expect(
      await screen.findByText("They asked you to stop, and a rep recorded it"),
    ).not.toBeNull();
  });

  // And the correspondence reason still shows. Without this the fix above
  // could be "show the last reason", which would take the row a reply rides
  // out with it.
  it("still says why correspondence is allowed", async () => {
    mount("research", view, [newsletter, correspondence]);

    expect(
      await screen.findByText("They said you may write to them"),
    ).not.toBeNull();
  });
});

// A contact somebody wrote in the system it was imported from.
//
// An import runs as one administrator, so `captured_by` names that seat on
// every row it wrote. That is true and useless as a statement about
// authorship. The author field knows, so the details prefer it. The timeline
// on this page already reads "Logged in HubSpot by …" while the details said
// "Typed by you", and the two disagreed side by side.
describe("a contact imported from somewhere else", () => {
  it("names who wrote it there, not the seat that ran the import", async () => {
    mount("research", {
      ...view,
      contact: {
        ...view.contact,
        captured_by: "human:u-1",
        author: { display_name: "Mutaz Suleiman", via: "HubSpot" },
      },
    });

    expect(
      await screen.findByText("Logged in HubSpot by Mutaz Suleiman"),
    ).toBeTruthy();
    expect(screen.queryByText("Typed by you")).toBeNull();
  });
});
