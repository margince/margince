/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AssistantPanel } from "../voice-knowledge/assistant";
import { CompanyScreen } from "./companies";
import { openProfile, render } from "./companies.testkit";
import {
  company,
  company360,
  companyBackstop,
  jsonResponse,
} from "./company.fixtures";
import { stubFetch } from "./company.testkit";
import { TodayOnThisAccount } from "./companytoday";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// One stalled-deal suggestion, as the 360 serves it. The reason is the part
// the rep judges, so the reason is what the tests assert on.
const stalledSuggestion = {
  kind: "stalled_deal",
  reason:
    '"Fleet retrofit 2026" has had no activity long enough to count as stalled.',
  fingerprint: "fp-stalled-1",
  subject_type: "deal",
  subject_id: "d-1",
  evidence: [{ entity_type: "deal", entity_id: "d-1" }],
};

// The suggestion rows, through the brief the record page mounts them in.
function renderSuggestionsFor(three60: unknown) {
  render(
    <TodayOnThisAccount
      companyId="o-1"
      view={three60 as never}
      loading={false}
      failed={false}
      onOpenRecord={() => {}}
      onPerform={() => {}}
    />,
  );
}

describe("CompanyScreen — next-step suggestions", () => {
  it("leads each suggestion with the reason the rule fired, and cites the record", async () => {
    const three60 = { ...company360, suggestions: [stalledSuggestion] };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    expect(screen.getByText("Stalled deal")).toBeTruthy();
    // The evidence is reachable: a suggestion the rep cannot check is a
    // verdict. It sits behind the reason rather than beside it, so checking
    // costs one gesture and reading the advice costs none.
    await userEvent.click(screen.getByText(stalledSuggestion.reason));
    expect(screen.getByRole("button", { name: "deal" })).toBeTruthy();
  });

  it("opens the composer on the message a draft-reply action names", async () => {
    // Through the page: the card only names the action, and the page is what
    // performs it.
    const unanswered = {
      ...stalledSuggestion,
      kind: "no_reply",
      fingerprint: "fp-reply-1",
      reason: "You reached out 15 days ago and nobody has come back.",
      action: { kind: "draft_reply", activity_id: "a-1" },
    };
    stubFetch(companyBackstop, {
      company360: { ...company360, suggestions: [unanswered] },
    });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    await waitFor(() =>
      expect(container.querySelector(".co-rail")).toBeTruthy(),
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Create draft" }),
    );
    // The composer stands over the record. The details pane it covers is
    // still where the reader left it.
    await screen.findByRole("button", { name: "Cancel" });
    expect(container.querySelector(".co-rail")).toBeTruthy();
  });

  it("goes to the deal an open-deal action names", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        suggestions: [
          {
            ...stalledSuggestion,
            action: { kind: "open_deal", deal_id: "d-7" },
          },
        ],
      },
    });
    render(<CompanyScreen id="o-1" />);
    await userEvent.click(
      await screen.findByRole("button", { name: "Open deal" }),
    );
    await waitFor(() => expect(window.location.hash).toContain("d-7"));
  });

  it("names how many suggestions the card left out", async () => {
    const three60 = {
      ...company360,
      suggestions: [stalledSuggestion],
      suggestions_dropped: 3,
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    // A truncated list with no count reads as "that is everything".
    await waitFor(() =>
      expect(screen.getByText("3 more not shown.")).toBeTruthy(),
    );
  });

  it("stays silent about what it left out when there is nothing left out", async () => {
    // Zero is the ordinary case, so the "N more" line must not render on it.
    // Otherwise every card carries "0 more not shown."
    const three60 = {
      ...company360,
      suggestions: [stalledSuggestion],
      suggestions_dropped: 0,
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    expect(screen.queryByText(/more not shown/)).toBeNull();
  });

  it("stays silent about what it left out when the count is absent", async () => {
    // Absent means the section was never computed. A "0 more" line would state a
    // fact about an account this read did not look at.
    const three60 = {
      ...company360,
      suggestions: [stalledSuggestion],
      suggestions_dropped: undefined,
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    expect(screen.queryByText(/more not shown/)).toBeNull();
  });

  it("offers no advice row when the account needs nothing", async () => {
    stubFetch(companyBackstop);
    renderSuggestionsFor(company360);

    await screen.findByRole("heading", { name: "Needs attention" });
    expect(screen.queryByRole("button", { name: "Not now" })).toBeNull();
  });

  it("names the withheld advice rather than claiming nothing needs attention", async () => {
    const three60 = {
      ...company360,
      suggestions: undefined,
      sections_omitted: ["suggestions"],
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await screen.findByText(/^Not included: suggestions\./);
    expect(screen.queryByText("Nothing needs attention right now.")).toBeNull();
  });

  it("dismisses by fingerprint and leaves the row for the server to remove", async () => {
    let dismissed: unknown;
    stubFetch(
      async (url, method, request) => {
        if (method === "POST" && url.includes("/suggestions/dismiss")) {
          dismissed = await request.json();
          return new Response(null, { status: 204 });
        }
        return companyBackstop(url);
      },
      { company360: { ...company360, suggestions: [stalledSuggestion] } },
    );
    renderSuggestionsFor({ ...company360, suggestions: [stalledSuggestion] });

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    await waitFor(() => expect(dismissed).toBeTruthy());
    // The server decides what survives: the card sends the fingerprint and
    // does not hide the row itself. Whether the surrounding page then re-reads
    // the 360 is the page's business, and this suite mounts the brief alone.
    expect(dismissed).toEqual({ fingerprint: "fp-stalled-1" });
    expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy();
  });

  it("says a dismissal failed instead of leaving the click looking like a miss", async () => {
    stubFetch(
      async (url, method) => {
        if (method === "POST" && url.includes("/suggestions/dismiss")) {
          return jsonResponse({ title: "nope" }, 500);
        }
        return companyBackstop(url);
      },
      { company360: { ...company360, suggestions: [stalledSuggestion] } },
    );
    renderSuggestionsFor({ ...company360, suggestions: [stalledSuggestion] });

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    await waitFor(() =>
      expect(screen.getByText(/was not dismissed/)).toBeTruthy(),
    );
    // The row is still there, which is what the notice is telling the reader.
    expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy();
  });
});

describe("CompanyScreen — Ask Margince", () => {
  const answer = {
    company_id: "o-1",
    question: "whats_open",
    generated_at: "2026-06-01T09:00:00Z",
    generated_by: "model",
    sentences: [
      {
        text: "Two open deals, worth about 57000 EUR.",
        evidence: [{ entity_type: "deal", entity_id: "d-1" }],
      },
    ],
  };

  it("asks only the prepared questions, and shows which one the answer answers", async () => {
    let asked: unknown;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.endsWith("/ask")) {
        asked = await request.json();
        return jsonResponse(answer);
      }
      return companyBackstop(url);
    });
    render(<AssistantPanel companyId="o-1" onOpenRecord={() => {}} />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "What is open here?" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "What is open here?" }),
    );

    await waitFor(() => expect(asked).toEqual({ question: "whats_open" }));
    await waitFor(() =>
      expect(
        screen.getByText("Two open deals, worth about 57000 EUR."),
      ).toBeTruthy(),
    );
    // Which writer produced it is never implied.
    expect(screen.getByText("Written by Margince")).toBeTruthy();
    // The question is repeated over its answer, so a reader who has scrolled
    // cannot pair the wrong one with it.
    expect(screen.getAllByText("What is open here?").length).toBeGreaterThan(1);
  });

  it("says there is nothing to answer from rather than nothing at all", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.endsWith("/ask")) {
        return jsonResponse({ ...answer, sentences: [] });
      }
      return companyBackstop(url);
    });
    render(<AssistantPanel companyId="o-1" onOpenRecord={() => {}} />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "What is open here?" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "What is open here?" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/No records you can access/)).toBeTruthy(),
    );
  });

  it("reports a failed question instead of leaving the card blank", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.endsWith("/ask")) {
        return jsonResponse({ title: "nope" }, 500);
      }
      return companyBackstop(url);
    });
    render(<AssistantPanel companyId="o-1" onOpenRecord={() => {}} />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "What is open here?" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "What is open here?" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/could not be answered/)).toBeTruthy(),
    );
  });
});

// One column beside the left rail, with the account's context in the right
// aside and tags and lists in that column's own panel. A layout change must
// not become an availability change.
describe("CompanyScreen — State D's one column and its card grid", () => {
  it("puts the account's context on the right, beside the work", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    await waitFor(() =>
      expect(container.querySelector(".co-overview-stack")).toBeTruthy(),
    );
    // The context is the record's details pane, beside the work under the tab
    // row. The record keeps no left rail: a second column would be a third
    // place to look.
    const pane = container.querySelector(".record-aside");
    expect(pane).toBeTruthy();
    expect(pane?.querySelector(".co-rail")).toBeTruthy();
    expect(container.querySelector(".record-rail")).toBeNull();
  });

  // Every card is on the page, wherever it sits. Each is named, not counted:
  // a count passes on a layout that lost one card and grew another. Each is
  // asserted where it lives: the pipeline on the Deals tab, the money on the
  // Finance tab, tags/lists in the context column.
  it("carries every panel of the overview stack, and files what is left in the context column", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    // The overview stack: what is worth doing and the pipeline's own figures.
    // The advice rows are the suggestions suite's above.
    const stack = container.querySelector(".co-overview-stack");
    expect(stack).toBeTruthy();
    // The money is a tab, so the overview column must not also carry it: a
    // figure in two places must be reconciled. Contract and pipeline figures,
    // projects and Finance each read on their own tab. "Commercial" also names
    // a health dimension in the brief, so the panel heading is what is checked.
    const headings = within(stack as HTMLElement)
      .getAllByRole("heading")
      .map((heading) => heading.textContent);
    expect(headings).not.toContain("Commercial");
    expect(stack?.textContent).not.toContain("Finance");
    expect(stack?.textContent).not.toContain("Lists & tags");

    // What is in flight reads on the Deals tab and the rail's DealsSection.
    // This fixture has none, so the fit card takes the overview's work slot:
    // whether to sell here at all differs from what is running today.
    expect(stack?.textContent).toContain("Growth fit");

    // What Margince spotted reads in the work column, beside the rest of what
    // wants a decision, rather than in the context column.
    expect(stack?.textContent).toContain("Signals");

    // The relationships and how the account is filed live in the page's
    // context column. That column is the shell's and is portalled out of this
    // record's tree, so it is queried off the document. The rail's summaries
    // stand on every tab, capped to their top rows.
    const rail = document.querySelector(".co-rail");
    expect(rail).toBeTruthy();
    for (const card of ["Active deals", "Key contacts", "Details", "Tags"]) {
      expect(rail?.textContent).toContain(card);
    }

    // The pipeline and the commercial picture have their own tab too. Named by
    // prefix: a tab carries the count of what is behind it, so its accessible
    // name is the label and the figure.
    await userEvent.click(screen.getByRole("button", { name: /^Deals/ }));
    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "Deals" })).toBeTruthy(),
    );
  });

  // Cards of unequal height never share a row: a tall needs list beside a
  // short Ask box leaves a gap. So the glance is one column at every width,
  // in the order a rep works it.
  it("stacks the glance in one column: what needs a contact, the money, what the account is, then the questions", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    const stack = container.querySelector(".co-overview-stack");
    expect(container.querySelector(".co-glance-cols")).toBeNull();
    if (!(stack instanceof HTMLElement)) {
      throw new Error("overview stack did not render");
    }

    // Each pane's own title, in DOM order.
    const headings = within(stack)
      .getAllByRole("heading")
      .map((heading) => heading.textContent);
    const needsAt = headings.indexOf("Needs attention");
    const dossierAt = headings.indexOf("Company overview");
    const askAt = headings.indexOf("Ask about this company");
    // The fit card takes this account's slot. Nothing in this fixture is in
    // flight, so the question is whether to sell here at all.
    const fitAt = headings.indexOf("Growth fit");
    expect(needsAt).toBeGreaterThanOrEqual(0);
    expect(dossierAt).toBeGreaterThan(needsAt);
    expect(askAt).toBeGreaterThan(dossierAt);
    expect(fitAt).toBeGreaterThan(askAt);
  });

  // The drawer opens over the record, not into a column of it. It is portalled
  // above a scrim and takes none of the page's width. So the details pane stays
  // where the reader left it, while the drawer stands and after it closes.
  it("leaves the details pane standing while a composer is open", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    await waitFor(() =>
      expect(container.querySelector(".co-rail")).toBeTruthy(),
    );

    await userEvent.click(screen.getByRole("button", { name: "Email" }));
    await screen.findByRole("button", { name: "Cancel" });
    expect(container.querySelector(".co-rail")).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull(),
    );
    expect(container.querySelector(".co-rail")).toBeTruthy();
  });

  // A call or a note often carries no subject. Counting only the subjected
  // rows would draw "nothing logged with them yet" on an account called five
  // times. That is a false statement about the account, made from a row.
  it("counts every logged activity, not only the ones with a subject", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        activities: {
          data: [
            {
              id: "a-1",
              kind: "call",
              occurred_at: "2026-06-01T08:30:00Z",
              direction: "inbound",
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    // Folded on arrival, naming how much it holds: that subjectless call
    // counts.
    const stack = container.querySelector(".co-overview-stack");
    const fold = stack?.querySelector("details");
    const summary = fold?.querySelector("summary");
    await waitFor(() => expect(summary?.textContent).toContain("Activity · 1"));
    expect(fold?.open).toBe(false);

    // Opened, the call is in it, not the sentence an account with nothing
    // logged would get.
    if (!summary) {
      throw new Error("the thread fold draws no summary to open it by");
    }
    await userEvent.click(summary);
    expect(fold?.open).toBe(true);
    expect(stack?.textContent).not.toContain("Nothing logged with them yet");
  });

  // A control that says only "show" makes a reader open it to find out whether
  // it was worth opening. The folded row carries the newest exchange, so the
  // decision is made before the click and the glance keeps its space.
  it("teases the newest exchange on the folded row, without opening it", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        activities: {
          data: [
            {
              id: "a-new",
              kind: "email",
              subject: "Rate sheet sent",
              occurred_at: "2026-06-02T09:00:00Z",
              direction: "outbound",
            },
            {
              id: "a-old",
              kind: "call",
              subject: "Quarterly review",
              occurred_at: "2026-06-01T08:30:00Z",
              direction: "inbound",
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    const fold = container
      .querySelector(".co-overview-stack")
      ?.querySelector("details");
    const summary = fold?.querySelector("summary");
    await waitFor(() =>
      expect(summary?.textContent).toContain("Rate sheet sent"),
    );

    // Teased, not opened: the row says what is under it while the section
    // stays folded.
    expect(fold?.open).toBe(false);
    // The newest, not the first the reader would meet scrolling.
    expect(summary?.textContent).not.toContain("Quarterly review");
  });

  // A name and a link say somebody exists. To pick which contact to call, a
  // reader would open all three to find the buyer. The list carries that.
  it("carries what a listed contact does and how to write to them", async () => {
    stubFetch(companyBackstop, {
      // The reader has a mailbox to send from, so the address is the
      // composer's; without one it would be their own mail client's.
      connectors: {
        data: [
          { id: "g1", provider: "gmail", status: "connected", scopes: [] },
        ],
      },
      company360: {
        ...company360,
        contacts: {
          data: [
            {
              contact_id: "p-1",
              full_name: "Anna Brandt",
              title: "Head of Procurement",
              primary_email: "anna.brandt@brandt-automotive.de",
              deal_roles: [],
              consent: {},
              strength: {
                score: 0,
                bucket: "none",
                factors: {
                  recency: 0,
                  frequency: 0,
                  reciprocity: 0,
                  direction: 0,
                },
              },
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    render(<CompanyScreen id="o-1" />);

    // The name opens the record and is the link's whole name. The mark beside
    // it draws initials as text, which would otherwise be announced first.
    // The page names this contact in the roster card and the references beside
    // it. Every one is asserted, since a single lookup misses a stray link.
    const names = await screen.findAllByRole("link", { name: "Anna Brandt" });
    for (const each of names) {
      expect(each.getAttribute("href")).toBe("#/contacts/p-1");
    }
    const name = names[0];
    expect(screen.getAllByText("Head of Procurement").length).toBeGreaterThan(
      0,
    );

    // The address is its own control, a sibling of the name, not nested in it.
    // A control inside a link is a press nobody can predict. It is a button
    // into the product's composer, never a link the reader's mail client takes.
    const write = (
      await screen.findAllByRole("button", {
        name: "anna.brandt@brandt-automotive.de",
      })
    )[0];
    expect(write.hasAttribute("href")).toBe(false);
    expect(name.contains(write)).toBe(false);
  });

  // None of the reference cards comes from the 360; each runs its own read.
  // So they must be on the page before that read lands, and stay if it never does.
  it("offers the reference cards before the 360 read lands", async () => {
    let releaseView: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      releaseView = resolve;
    });
    const { fetchMock } = stubFetch(companyBackstop);
    const held360 = vi.fn(async (request: Request) => {
      if (new URL(request.url).pathname.endsWith("/360")) {
        await held;
      }
      return fetchMock(request);
    });
    vi.stubGlobal("fetch", held360);

    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    await openProfile();

    // Asserted on the panel headings the tab is built from. These words also
    // appear in the app's navigation, so a bare text match would pass on a
    // page that lost every one of these sections.
    const headings = () =>
      Array.from(container.querySelectorAll(".panel-head")).map((el) =>
        el.textContent?.trim(),
      );
    // A panel's header band carries its description line and its title.
    // So these match the title at the start, not the whole band.
    const opensWith = (title: string) =>
      headings().some((one) => one?.startsWith(title));
    await waitFor(() => expect(opensWith("Details")).toBe(true));
    expect(opensWith("Description")).toBe(true);
    expect(opensWith("Company facts")).toBe(true);
    // The roll-up is one of the panes that runs its own read, so it arrives
    // on its own clock rather than with the ones above.
    await waitFor(() => expect(opensWith("Roll-up")).toBe(true));
    releaseView?.();
  });
});
