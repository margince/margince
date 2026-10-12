/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { holdExits } from "../design-system/presence-testing";
import { LocaleProvider } from "../i18n";
import { steppedClock } from "../testing/steppedclock";
import { type GrantSpec, meFixture } from "./mefixture";
import { CREATE_ID } from "./nav";
import {
  type Command,
  CommandPalette,
  paletteHotkeyCaps,
  useBuiltinCommands,
  usePaletteHotkey,
} from "./palette";

// B-EP09.5 (AC-shell-3..7) and RS-1 (live /search records + see-all)
// acceptance. AC-shell-8 is no longer about this file: it covered the
// record-scoped Ask composer, and now covers the claim that composer carried
// about what the agent can reach — asserted on the panel that carries it, in
// frontend/e2e/ac.spec.ts and agentrail.scope.test.tsx.

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function wrap(ui: ReactNode, client: QueryClient) {
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>
  );
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = rtlRender(wrap(ui, client));
  return {
    ...view,
    rerender: (next: ReactNode) => view.rerender(wrap(next, client)),
  };
};

const commands: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    keywords: ["pipeline"],
    type: "screen",
    route: { screen: "deals" },
  },
  {
    id: "action:new-deal",
    label: "New deal",
    type: "action",
    route: { screen: "deals", id: CREATE_ID },
  },
  {
    id: "record:brandt",
    label: "Brandt Automotive",
    subtitle: "Company",
    type: "record",
    route: { screen: "companies", id: "brandt" },
  },
];

// The palette answers to both modifiers, but the affordance may advertise only
// one, and ⌘ names a key a Windows keyboard does not have.
// The DESTINATIONS, which is what every assertion below means by a row. Asking
// sits above them and is not one: it answers a different question and is not
// something the arrow keys walk, so a test that counted it would be asserting
// about a control it never meant.
function destinationRows(): HTMLElement[] {
  return screen
    .getAllByRole("button")
    .filter((button) => !button.classList.contains("palette-ask"));
}

describe("paletteHotkeyCaps", () => {
  it("names the modifier the platform actually has", () => {
    expect(paletteHotkeyCaps("MacIntel")).toEqual(["⌘", "K"]);
    expect(paletteHotkeyCaps("iPhone")).toEqual(["⌘", "K"]);
    expect(paletteHotkeyCaps("Win32")).toEqual(["Ctrl", "K"]);
    expect(paletteHotkeyCaps("Linux x86_64")).toEqual(["Ctrl", "K"]);
  });

  // An unreported platform is far more likely to be Windows or Linux than a Mac,
  // and Ctrl is the modifier that works on both.
  it("falls back to Ctrl when the platform is unknown", () => {
    expect(paletteHotkeyCaps("")).toEqual(["Ctrl", "K"]);
  });

  // One cap per key, and never a string a caller has to take apart again: the
  // surface that draws these drew them by splitting "⌘K" with a lookbehind regex,
  // which does not parse at all on an engine without lookbehind.
  it("hands back one entry per key, so no caller has to split a string", () => {
    for (const platform of ["MacIntel", "Win32", ""]) {
      expect(paletteHotkeyCaps(platform)).toHaveLength(2);
    }
  });
});

describe("CommandPalette (AC-shell-3/4/5/6)", () => {
  it("shows the default command list with type tags, focuses the input", () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    expect(document.activeElement).toBe(screen.getByRole("searchbox"));
    expect(screen.getByText("Deals")).toBeTruthy();
    expect(screen.getByText("Record")).toBeTruthy(); // type tag rendered
  });

  // A nav label is a presentation choice; the word a reader already learned
  // outlives it. Typing "pipeline" has to reach the Deals row, or relabelling a
  // destination quietly removes it from the palette for everyone who knows it by
  // its older name.
  it("matches a keyword the row does not display, without showing it", async () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "pipeline");
    const rows = destinationRows();
    expect(rows[0].textContent).toContain("Deals");
    expect(rows[0].textContent).not.toContain("pipeline");
    await userEvent.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
  });

  it("filters by label+subtitle case-insensitively and puts the see-all row after them", async () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "COMPANY");
    const rows = destinationRows();
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain("Brandt Automotive");
    expect(rows[1].textContent).toContain("See all results");
    // Asking is not among them, and is offered whatever was typed.
    expect(screen.getByText("Ask your documents")).toBeTruthy();
  });

  it("Enter runs the selection; arrows move and clamp (AC-shell-5)", async () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    const input = screen.getByRole("searchbox");
    await userEvent.keyboard("{ArrowUp}"); // clamps at 0
    await userEvent.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}"); // clamps at end
    await userEvent.keyboard("{ArrowUp}{ArrowUp}"); // back to index 0
    expect(input).toBeTruthy();
    await userEvent.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
  });

  // Asking is offered ABOVE the destinations rather than among them, because it
  // answers a different question and because the first row is the row Enter
  // presses: pinned into the list it would have asked about a screen name a
  // reader typed on their way to that screen. It carries what is in the box
  // without asking it — a question matched mid-word is one still being written.
  it("opens the dialog where the reader is, carrying the query and taking them nowhere", async () => {
    window.location.hash = "#/deals";
    const onClose = vi.fn();
    render(<CommandPalette open onClose={onClose} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "zzz nothing matches");
    await userEvent.click(screen.getByText("Ask your documents"));
    // The screen the reader was on is still the screen they are on.
    // Two dials: presence opens it, the question rides beside it. An empty
    // dial does not survive parseParams, so "open with an empty box" needs a
    // dial of its own rather than an empty value.
    expect(window.location.hash).toBe("#/deals?ask=1&askq=zzz+nothing+matches");
    // And nowhere else. The address is the whole carrier, so there is no
    // second copy for a reader's next tab to inherit.
    expect(sessionStorage.length).toBe(0);
    expect(onClose).toHaveBeenCalled();
  });

  // Enter still belongs to the destinations: a reader who typed a screen name
  // and pressed it goes there, and asking is not what they did.
  it("Enter runs the first destination, not the ask", async () => {
    window.location.hash = "#/home";
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "deals");
    await userEvent.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
  });

  it("closes on a selection that lands on the address it is already at", async () => {
    window.location.hash = "#/deals";
    const onClose = vi.fn();
    render(<CommandPalette open onClose={onClose} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "deals");
    await userEvent.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
    expect(onClose).toHaveBeenCalled();
  });

  it("Esc closes; opening clears the previous query (AC-shell-3)", async () => {
    const onClose = vi.fn();
    const view = render(
      <CommandPalette open onClose={onClose} commands={commands} />,
    );
    await userEvent.type(screen.getByRole("searchbox"), "deal");
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
    view.rerender(
      <CommandPalette open={false} onClose={onClose} commands={commands} />,
    );
    view.rerender(
      <CommandPalette open onClose={onClose} commands={commands} />,
    );
    expect((screen.getByRole("searchbox") as HTMLInputElement).value).toBe("");
  });

  // The palette is a dialog and had none of a dialog's keyboard behaviour: it
  // drew its own box, so it grew its own answer, and the answer was Escape on
  // the search input alone plus no Tab trap. Both are `useDialogFocus`'s now,
  // and both are asserted here rather than left to the browser to find again.

  it("Esc closes from a result row, not only from the search box", async () => {
    const onClose = vi.fn();
    render(<CommandPalette open onClose={onClose} commands={commands} />);
    const user = userEvent.setup();
    // Where the arrow keys put a reader — and where Escape used to do nothing,
    // because the handler belonged to the input this focus has left.
    const row = screen.getByRole("button", { name: /Deals/ });
    row.focus();
    expect(document.activeElement).toBe(row);
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });

  it("keeps Tab inside the dialog in both directions", async () => {
    const outside = document.createElement("a");
    outside.href = "#/somewhere-behind";
    outside.textContent = "a link on the page behind";
    document.body.append(outside);
    try {
      render(<CommandPalette open onClose={() => {}} commands={commands} />);
      const user = userEvent.setup();
      const dialog = screen.getByRole("dialog");
      screen.getByRole("searchbox").focus();

      // Backwards off the first stop was the reported failure: one Shift+Tab
      // and focus was on the page behind.
      await user.tab({ shift: true });
      expect(dialog.contains(document.activeElement)).toBe(true);

      // And forwards off the last, which is the same wrap in the other
      // direction. Tab enough times to pass every row the palette drew.
      for (let step = 0; step < commands.length + 3; step += 1) {
        await user.tab();
        expect(dialog.contains(document.activeElement)).toBe(true);
      }
    } finally {
      outside.remove();
    }
  });

  it("hands focus back to whatever opened it", async () => {
    const opener = document.createElement("button");
    opener.textContent = "open the palette";
    document.body.append(opener);
    try {
      opener.focus();
      const view = render(
        <CommandPalette open onClose={() => {}} commands={commands} />,
      );
      expect(document.activeElement).not.toBe(opener);
      view.rerender(
        <CommandPalette open={false} onClose={() => {}} commands={commands} />,
      );
      await waitFor(() => expect(document.activeElement).toBe(opener));
    } finally {
      opener.remove();
    }
  });

  it("surfaces live record hits from /search plus a see-all row (RS-1)", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [{ type: "contact", id: "p1", title: "Dana Buyer at Acme" }],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "acme");
    await waitFor(() =>
      expect(screen.getByText("Dana Buyer at Acme")).toBeTruthy(),
    );
    expect(screen.getByText("See all results for “acme”")).toBeTruthy();

    await user.click(screen.getByText("Dana Buyer at Acme"));
    expect(window.location.hash).toBe("#/contacts/p1");
  });

  // A failed record search used to answer with an empty list, which is the
  // same shape as "no matches" — so a reader whose search 500'd was told the
  // workspace holds nothing. It says what happened now, and the builtin
  // commands stay usable beside it, which is the degradation that was wanted.
  it("says the record search failed rather than reporting an empty workspace", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("nope", { status: 500 })),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "acme");

    expect(await screen.findByText(/Search failed/)).toBeTruthy();
    // Not the empty state: the list is not empty, and saying so would be the
    // false claim this replaces.
    expect(screen.queryByText("No matches.")).toBeNull();
    // The builtin half still answers. Retyped, because the failing query was
    // "acme" and no builtin command carries that word — the claim is that a
    // failed RECORD search leaves the command list working, not that it leaves
    // the previous query matching something it never matched.
    await user.clear(screen.getByRole("searchbox"));
    await user.type(screen.getByRole("searchbox"), "Deals");
    expect(
      destinationRows().some((row) => row.textContent?.includes("Deals")),
    ).toBe(true);
  });

  // A hit's kind is the heading of its group, in the reader's words. It used to
  // be the row's own second line, and before that the untranslated wire word.
  it("names a hit's kind in the reader's language, not the wire's", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [{ type: "company", id: "o1", title: "Brandt GmbH" }],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    const { container } = render(
      <CommandPalette open onClose={() => {}} commands={commands} />,
    );
    await user.type(screen.getByRole("searchbox"), "brandt");
    const row = await screen.findByRole("button", { name: /Brandt GmbH/ });
    expect(
      [...container.querySelectorAll(".palette-group")].map(
        (heading) => heading.textContent,
      ),
    ).toEqual(["Companies"]);
    // The heading says it once; the row repeats neither the kind nor "Record".
    expect(row.querySelector(".label")?.textContent).toBe("Brandt GmbH");
    expect(row.querySelector(".sub")).toBeNull();
    expect(row.querySelector(".badge")).toBeNull();
  });

  // The fix this grouping exists for: a word that names an account also names
  // every thread about it, and a short list ranked across kinds was all mail.
  // The palette asks for a few of each kind and draws the account first.
  it("draws the account above the mail that outranks it, asking for a few of each kind", async () => {
    const user = steppedClock();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) =>
      jsonResponse({
        data: [
          {
            type: "activity",
            id: "a1",
            title: "Re: Acme renewal",
            score: 8,
            email_summary: {
              activity_id: "a1",
              subject: "Re: Acme renewal",
              occurred_at: "2026-09-01T09:15:00Z",
              counterparty: "Dana Buyer",
            },
          },
          { type: "company", id: "o1", title: "Acme GmbH", score: 2 },
        ],
        page: { next_cursor: null, has_more: false },
        types_with_more: ["activity"],
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const { container } = render(
      <CommandPalette open onClose={() => {}} commands={commands} />,
    );
    await user.type(screen.getByRole("searchbox"), "acme");

    const email = await screen.findByRole("button", { name: /Acme renewal/ });
    expect(
      [...container.querySelectorAll(".palette-group")].map(
        (heading) => heading.textContent,
      ),
    ).toEqual(["Companies", "Emails"]);
    const rows = destinationRows().map((row) => row.textContent ?? "");
    expect(rows.findIndex((text) => text.includes("Acme GmbH"))).toBeLessThan(
      rows.findIndex((text) => text.includes("Acme renewal")),
    );
    // Cited the way every surface cites a message: its subject and its date.
    expect(email.querySelector(".emailref__subject")?.textContent).toBe(
      "Re: Acme renewal",
    );
    expect(email.querySelector(".emailref__when")?.textContent).toBeTruthy();

    const asked = fetchMock.mock.calls.map(([input]) =>
      input instanceof Request ? input.url : String(input),
    );
    expect(asked.some((url) => url.includes("per_type=3"))).toBe(true);
  });

  // A partner is a property of a company rather than a kind of its own, so the
  // account sits with the companies and its second line says it is a partner.
  it("names a partner company as one on its second line", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "company",
              id: "o1",
              title: "Brandt GmbH",
              is_partner: true,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "brandt");
    const row = await screen.findByRole("button", { name: /Brandt GmbH/ });
    expect(row.querySelector(".sub")?.textContent).toBe("Partner");
  });

  // A contact found through the company the word named says so, or the
  // reader meets a name with no reason it is in the list. The palette asks
  // for those contacts, and the row draws the record's own mark.
  it("says which matched company a contact works at, under its mark", async () => {
    const user = steppedClock();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) =>
      jsonResponse({
        data: [
          {
            type: "contact",
            id: "p1",
            title: "Jonas Weiß",
            works_at: { company_id: "o1", company_name: "Acme GmbH" },
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "acme");

    const row = await screen.findByRole("button", {
      name: "Jonas Weiß Works at Acme GmbH",
    });
    expect(row.querySelector(".sub")?.textContent).toBe("Works at Acme GmbH");
    // The mark is hidden from the row's name, which is why the name above
    // carries no initials.
    const mark = row.querySelector(".palette-mark");
    expect(mark?.getAttribute("aria-hidden")).toBe("true");
    expect(mark?.querySelector(".avatar")?.textContent).toBe("JW");
    const asked = fetchMock.mock.calls.map(([input]) =>
      input instanceof Request ? input.url : String(input),
    );
    expect(asked.some((url) => url.includes("with_employees=true"))).toBe(true);
  });

  it("draws a company's logo on its mark", async () => {
    const user = steppedClock();
    const logo =
      "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            { type: "company", id: "o1", title: "Acme GmbH", logo_url: logo },
            { type: "deal", id: "d1", title: "Acme renewal" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "acme");

    const company = await screen.findByRole("button", { name: "Acme GmbH" });
    expect(
      company.querySelector(".palette-mark .avatar-img")?.getAttribute("src"),
    ).toBe(logo);
    // A deal is not a contact or a company, and keeps the row's glyph.
    const deal = screen.getByRole("button", { name: "Acme renewal" });
    expect(deal.querySelector(".palette-mark")).toBeNull();
  });

  // The marker means nothing off a company, so a hit of another kind draws no
  // partner line whatever the server sent beside it.
  it("draws no partner line on a non-company hit despite a partner marker", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "contact",
              id: "p1",
              title: "Dana Buyer",
              is_partner: true,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "dana");
    const row = await screen.findByRole("button", { name: /Dana Buyer/ });
    expect(row.querySelector(".sub")).toBeNull();
  });

  // A catalog row has no page of its own — it lives on the data-model settings
  // page — so that is where the hit goes. Being findable at all is the change;
  // an address of its own is worth having and is not this one.
  it("opens the catalog page from a product hit", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [{ type: "product", id: "pr1", title: "Floor scrubber" }],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "scrub");
    await waitFor(() =>
      expect(screen.getByText("Floor scrubber")).toBeTruthy(),
    );
    await user.click(screen.getByText("Floor scrubber"));
    // `fields`, the current spelling of the entry that carries products today.
    // The catalog splits it into fields/tags/products; this follows when the
    // screen renders those pages rather than the combined one.
    expect(window.location.hash).toContain("fields");
  });

  // Two projects called "Rollout" are told apart by their key and their
  // account, and the server sends exactly that as the hit's snippet — already
  // gated on whether the reader may see the account. The palette used to read
  // each project and its company again to build the same line.
  it("routes a project hit to its page, with its key and account as its line", async () => {
    const user = steppedClock();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) =>
      jsonResponse({
        data: [
          {
            type: "project",
            id: "pr-1",
            title: "Rollout",
            snippet: "ACME-CRM · Acme GmbH",
          },
          {
            type: "project",
            id: "pr-2",
            title: "Rollout",
            snippet: "Brandt Automotive",
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "roll");
    expect(await screen.findByText("ACME-CRM · Acme GmbH")).toBeTruthy();
    expect(screen.getByText("Brandt Automotive")).toBeTruthy();
    expect(
      fetchMock.mock.calls.some(([input]) =>
        String(input instanceof Request ? input.url : input).includes(
          "/projects/",
        ),
      ),
    ).toBe(false);

    await user.click(screen.getByText("ACME-CRM · Acme GmbH"));
    expect(window.location.hash).toBe("#/projects/pr-1");
  });

  // Typing a word and being taken to what carries it is the whole point of a
  // tag. The palette used to drop tag hits with the types that have no page —
  // a tag has one, so a searcher was left with no autocomplete for the
  // vocabulary at all.
  it("offers a tag hit and opens its page", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [{ type: "tag", id: "t-1", title: "Key Account" }],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await user.type(screen.getByRole("searchbox"), "key");
    await waitFor(() => expect(screen.getByText("Key Account")).toBeTruthy());

    await user.click(screen.getByText("Key Account"));
    expect(window.location.hash).toBe("#/tags/t-1");
  });
});

// The builtin set is DERIVED from the rail's destinations, so a screen with a
// rail row is a ⌘K command with no registration of its own — and a screen with
// neither is reachable only by typing its hash. That derivation is what these
// assert, end to end: the word a reader types, and the address they land on.
// The palette is not a `Modal` — it draws its own box — but it keeps the same
// two contracts every dialog here keeps, and this is the second one: it stays
// on the page while its exit plays, and while it does it is a picture of a
// palette and nothing a reader or a pointer can reach.
describe("a palette that is leaving", () => {
  it("stays on the page, inert and out of the accessibility tree", () => {
    const exit = new Promise<void>(() => undefined);
    const animations = vi
      .spyOn(HTMLElement.prototype, "getAnimations")
      // The two properties `usePresence` reads: whether this animation can end
      // at all, and when it did.
      .mockReturnValue([
        {
          finished: exit,
          effect: { getComputedTiming: () => ({ iterations: 1 }) },
        } as unknown as Animation,
      ]);
    try {
      const onClose = vi.fn();
      const { baseElement, rerender } = render(
        <CommandPalette open onClose={onClose} commands={commands} />,
      );
      rerender(
        <CommandPalette open={false} onClose={onClose} commands={commands} />,
      );

      const overlay = baseElement.querySelector(".palette-overlay");
      expect(overlay?.getAttribute("data-state")).toBe("closing");
      expect(overlay?.hasAttribute("inert")).toBe(true);
      expect(overlay?.getAttribute("aria-hidden")).toBe("true");
      expect(screen.queryByRole("dialog")).toBeNull();
    } finally {
      animations.mockRestore();
    }
  });

  it("renders nothing at all where nothing animates", () => {
    // Reduced motion, and jsdom: no animation means nothing to wait for, so the
    // palette goes on the render that dismissed it, exactly as it did before it
    // had an exit.
    const { baseElement, rerender } = render(
      <CommandPalette open onClose={() => {}} commands={commands} />,
    );
    rerender(
      <CommandPalette open={false} onClose={() => {}} commands={commands} />,
    );
    expect(baseElement.querySelector(".palette-overlay")).toBeNull();
  });
});

// The shell's wiring, with a dialog the page behind may be holding up.
function ShellWithDialog({
  dialogOpen,
  onDialogClose = () => {},
}: Readonly<{ dialogOpen: boolean; onDialogClose?: () => void }>) {
  const [paletteOpen, setPaletteOpen] = useState(false);
  usePaletteHotkey(paletteOpen, setPaletteOpen);
  return (
    <>
      <CommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        commands={commands}
      />
      <Modal
        open={dialogOpen}
        onClose={onDialogClose}
        labelledBy="edit-deal"
        intent="form"
      >
        <Heading size="large" id="edit-deal">
          Edit deal
        </Heading>
        <button type="button">Save</button>
      </Modal>
    </>
  );
}

// An open dialog makes the rest of the app unreachable, and the palette is the
// rest of the app: raised over one it sat under the dialog's keyboard, so
// Escape closed the dialog and left the palette standing.
describe("the palette hotkey", () => {
  it("does nothing while a dialog is up, and Escape still closes the dialog", async () => {
    const onDialogClose = vi.fn();
    render(<ShellWithDialog dialogOpen onDialogClose={onDialogClose} />);
    const user = userEvent.setup();

    await user.keyboard("{Meta>}k{/Meta}");
    expect(screen.queryByRole("searchbox")).toBeNull();
    // The browser's own ⌘K is still withheld: the chord belongs to the product.
    expect(fireEvent.keyDown(document.body, { key: "k", metaKey: true })).toBe(
      false,
    );

    await user.keyboard("{Escape}");
    expect(onDialogClose).toHaveBeenCalledOnce();
  });

  it("opens the palette and closes it again on the same chord", async () => {
    render(<ShellWithDialog dialogOpen={false} />);
    const user = userEvent.setup();

    await user.keyboard("{Meta>}k{/Meta}");
    expect(screen.getByRole("searchbox")).toBeTruthy();
    await user.keyboard("{Meta>}k{/Meta}");
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("opens the palette again while its own exit is still playing", async () => {
    const exits = holdExits();
    try {
      render(<ShellWithDialog dialogOpen={false} />);
      const user = userEvent.setup();
      await user.keyboard("{Meta>}k{/Meta}");
      await user.keyboard("{Escape}");
      expect(document.querySelector(".palette-overlay[inert]")).not.toBeNull();

      await user.keyboard("{Meta>}k{/Meta}");
      const input = screen.getByRole("searchbox");
      expect(input.closest("[inert]")).toBeNull();
    } finally {
      exits.mockRestore();
    }
  });

  it("opens over a dialog that is already on its way out", async () => {
    const exits = holdExits();
    try {
      const view = render(<ShellWithDialog dialogOpen />);
      view.rerender(<ShellWithDialog dialogOpen={false} />);
      expect(view.baseElement.querySelector("#edit-deal")).not.toBeNull();
      const user = userEvent.setup();

      await user.keyboard("{Meta>}k{/Meta}");
      expect(screen.getByRole("searchbox")).toBeTruthy();
    } finally {
      exits.mockRestore();
    }
  });
});

describe("useBuiltinCommands", () => {
  function Probe() {
    return (
      <CommandPalette open onClose={() => {}} commands={useBuiltinCommands()} />
    );
  }

  // A /me with no grant at all, which is the harder case: the two settings
  // shortcuts drop out, so anything still offered here is offered because the
  // rail names it rather than because the principal holds something.
  function renderProbe() {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(meFixture({ roles: [], allow: {} }))),
    );
    return render(<Probe />);
  }

  // Company profile opens on the catalog's installation grants, and the profile
  // card on it is the admin's alone. A `company` grant opens nothing here,
  // whatever the installation says — the palette reads the rail's table.
  function renderProbeWithCompany(opts: { roles: string[]; allow: GrantSpec }) {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(meFixture({ roles: opts.roles, allow: opts.allow })),
      ),
    );
    return render(<Probe />);
  }
  const INSTALLATION_WRITER: GrantSpec = {
    installation_settings: ["read", "update"],
  };

  it("offers the company shortcut to the installation's writer", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({ roles: ["ops"], allow: INSTALLATION_WRITER });
    // Typed by its OLD name, which the palette keeps as a keyword: the page is
    // "Company profile" now, and a reader who learnt "General" should still
    // find it rather than concluding it was removed.
    await user.type(screen.getByRole("searchbox"), "general");
    await waitFor(() => {
      expect(destinationRows()[0].textContent).toContain("Company profile");
    });
  });

  it("withholds it from a company writer while the rollout is on", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({
      roles: ["rep"],
      allow: {
        company: ["create", "read", "update"],
        // The witness: Capture rules proves /me has answered before the
        // absence below is read.
        capture_settings: ["read"],
      },
    });
    const box = screen.getByRole("searchbox");
    await user.type(box, "capture");
    await screen.findByText("Capture rules");
    await user.clear(box);
    await user.type(box, "general");
    expect(screen.queryByText("Company profile")).toBeNull();
  });

  // Reading the company's own website is Company profile's job, so no seat is
  // offered a separate action for it.
  it.each([["admin"], ["ops"]])(
    "offers %s no separate read-a-company action",
    async (role) => {
      const user = userEvent.setup();
      renderProbeWithCompany({ roles: [role], allow: INSTALLATION_WRITER });
      await user.type(screen.getByRole("searchbox"), "company");
      await screen.findByText("Company profile");
      expect(screen.queryByText("Read a company")).toBeNull();
    },
  );

  it("reaches Company profile by its refresh button's words for an admin", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({ roles: ["admin"], allow: INSTALLATION_WRITER });
    await user.type(screen.getByRole("searchbox"), "website");
    await waitFor(() => {
      expect(destinationRows()[0].textContent).toContain("Company profile");
    });
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/settings/company");
  });

  // Ops reaches Company profile for the installation and the rates, and its
  // copy of the page draws no website card, so "website" leads it nowhere.
  it("does not send ops to Company profile for the website", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({ roles: ["ops"], allow: INSTALLATION_WRITER });
    const box = screen.getByRole("searchbox");
    await user.type(box, "company");
    await screen.findByText("Company profile");
    await user.clear(box);
    await user.type(box, "website");
    expect(screen.queryByText("Company profile")).toBeNull();
  });

  // The two destinations that carry a word the rail no longer prints. A reader
  // who learned "Contacts" or "Pipeline" types it, and the row it named must be
  // what answers — against the REAL rail rows, because the alias lives on the
  // nav item and a fixture command list would only prove the fixture.
  it.each([
    ["contacts", "Contacts", "#/contacts"],
    ["pipeline", "Deals", "#/deals"],
  ])(
    "reaches %s's destination by the name it used to print",
    async (typed, label, hash) => {
      const user = userEvent.setup();
      renderProbe();
      await user.type(screen.getByRole("searchbox"), typed);
      const rows = destinationRows();
      expect(rows[0].textContent).toContain(label);
      await user.keyboard("{Enter}");
      expect(window.location.hash).toBe(hash);
    },
  );

  it("reaches the filter builder by the name the screen prints", async () => {
    const user = userEvent.setup();
    renderProbe();
    // Its own title, not a fourth spelling of it: "views" appears in no other
    // command, so matching on it proves the row carries the screen's own words.
    await user.type(screen.getByRole("searchbox"), "views");
    const rows = destinationRows();
    expect(rows[0].textContent).toContain("Filters and views");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/filters");
  });

  it("reaches it by its route id too, so the domain word finds it", async () => {
    const user = userEvent.setup();
    renderProbe();
    await user.type(screen.getByRole("searchbox"), "filters");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/filters");
  });

  // The scheduled queue is off the rail deliberately, so the rail-derived rows
  // above cannot carry it and nothing else did: it was reachable only by typing
  // the address, while a rep sat on a message they wanted back.
  it("reaches the scheduled queue, which no rail row names", async () => {
    const user = userEvent.setup();
    renderProbe();
    await user.type(screen.getByRole("searchbox"), "scheduled");
    const rows = destinationRows();
    expect(rows[0].textContent).toContain("Scheduled messages");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/scheduled");
  });

  // The words a rep would actually type: they think of the CONTROL they used,
  // not of the destination. The alias is that control's own label, so it is
  // translated with it — English prose here would be words a German or
  // Vietnamese reader would never type.
  it("finds it under the words the composer's control uses", async () => {
    const user = userEvent.setup();
    renderProbe();
    await user.type(screen.getByRole("searchbox"), "Schedule send");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/scheduled");
  });
});
