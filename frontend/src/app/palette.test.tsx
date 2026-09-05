/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "./mefixture";
import {
  ASK_QUERY_KEY,
  type Command,
  CommandPalette,
  paletteHotkeyCaps,
  useBuiltinCommands,
} from "./palette";

// B-EP09.5 (AC-shell-3..7) and RS-1 (live /search records + see-all)
// acceptance. B-EP09.6 (AC-shell-8) covered the record-scoped Ask composer,
// which the agent surfaces that carried it no longer offer.

afterEach(() => {
  cleanup();
  window.location.hash = "";
  sessionStorage.clear();
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
    label: "Pipeline",
    keywords: ["deals"],
    type: "screen",
    route: { screen: "deals" },
  },
  {
    id: "action:new-deal",
    label: "New deal",
    type: "action",
    route: { screen: "deals", id: "new" },
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
    expect(screen.getByText("Pipeline")).toBeTruthy();
    expect(screen.getByText("Record")).toBeTruthy(); // type tag rendered
  });

  // A nav label is a presentation choice; the domain word outlives it. Typing
  // "deals" has to reach Pipeline, or renaming a destination quietly removes it
  // from the palette for everyone who knows it by its older name.
  it("matches a keyword the row does not display, without showing it", async () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "deals");
    const rows = screen.getAllByRole("button");
    expect(rows[0].textContent).toContain("Pipeline");
    expect(rows[0].textContent).not.toContain("deals");
    await userEvent.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/deals");
  });

  it("filters by label+subtitle case-insensitively and appends the see-all + Ask-AI rows last", async () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "COMPANY");
    const rows = screen.getAllByRole("button");
    expect(rows).toHaveLength(3);
    expect(rows[0].textContent).toContain("Brandt Automotive");
    expect(rows[1].textContent).toContain("See all results");
    expect(rows[2].textContent).toContain("Ask AI");
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

  it("the Ask-AI row stores the query and lands on the AI surface (AC-shell-4)", async () => {
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "zzz nothing matches");
    // rows are [see-all, ask-ai] here (no builtin/record matches): step past
    // the see-all row to reach Ask-AI.
    await userEvent.keyboard("{ArrowDown}");
    await userEvent.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/ai");
    expect(sessionStorage.getItem(ASK_QUERY_KEY)).toBe("zzz nothing matches");
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
    const row = screen.getByRole("button", { name: /Pipeline/ });
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
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [{ type: "person", id: "p1", title: "Dana Buyer at Acme" }],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "acme");
    await waitFor(() =>
      expect(screen.getByText("Dana Buyer at Acme")).toBeTruthy(),
    );
    expect(screen.getByText("See all results for “acme”")).toBeTruthy();

    await userEvent.click(screen.getByText("Dana Buyer at Acme"));
    expect(window.location.hash).toBe("#/contacts/p1");
  });

  // A failed record search used to answer with an empty list, which is the
  // same shape as "no matches" — so a reader whose search 500'd was told the
  // workspace holds nothing. It says what happened now, and the builtin
  // commands stay usable beside it, which is the degradation that was wanted.
  it("says the record search failed rather than reporting an empty workspace", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("nope", { status: 500 })),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "acme");

    expect(
      await screen.findByText(/Records could not be searched/),
    ).toBeTruthy();
    // Not the empty state: the list is not empty, and saying so would be the
    // false claim this replaces.
    expect(screen.queryByText("No matches.")).toBeNull();
    // The builtin half still answers. Retyped, because the failing query was
    // "acme" and no builtin command carries that word — the claim is that a
    // failed RECORD search leaves the command list working, not that it leaves
    // the previous query matching something it never matched.
    await userEvent.clear(screen.getByRole("searchbox"));
    await userEvent.type(screen.getByRole("searchbox"), "Pipeline");
    expect(
      screen
        .getAllByRole("button")
        .some((row) => row.textContent?.includes("Pipeline")),
    ).toBe(true);
  });

  // A row's second line names the kind, and it used to name it in the WIRE's
  // words — the untranslated enum member, straight onto the row.
  it("names a hit's kind in the reader's language, not the wire's", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [{ type: "organization", id: "o1", title: "Brandt GmbH" }],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "brandt");
    const row = await screen.findByRole("button", { name: /Brandt GmbH/ });
    // The row's OWN second line, read exactly: asserting on page text would
    // pass off the fixture command list's subtitle, and asserting `contains`
    // would pass on the wire word itself once the label is capitalised.
    expect(row.querySelector(".sub")?.textContent).toBe("Organization");
  });

  // A catalog row has no page of its own — it lives on the data-model settings
  // page — so that is where the hit goes. Being findable at all is the change;
  // an address of its own is worth having and is not this one.
  it("opens the catalog page from a product hit", async () => {
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
    await userEvent.type(screen.getByRole("searchbox"), "scrub");
    await waitFor(() =>
      expect(screen.getByText("Floor scrubber")).toBeTruthy(),
    );
    await userEvent.click(screen.getByText("Floor scrubber"));
    expect(window.location.hash).toContain("data-model");
  });

  // A project hit carries no snippet, and "project" under two projects both
  // called Rollout tells them apart by nothing. The row reads the project
  // itself for its key, and falls back to the company for a project without
  // one; the hit routes to the project page either way.
  it("routes a project hit to its page, with the key or the company as its line", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/search")) {
          return jsonResponse({
            data: [
              { type: "project", id: "pr-1", title: "Rollout", snippet: null },
              { type: "project", id: "pr-2", title: "Rollout", snippet: null },
            ],
            page: { next_cursor: null, has_more: false },
          });
        }
        if (url.endsWith("/projects/pr-1")) {
          return jsonResponse({ id: "pr-1", name: "Rollout", key: "ACME-CRM" });
        }
        if (url.endsWith("/projects/pr-2")) {
          return jsonResponse({
            id: "pr-2",
            name: "Rollout",
            key: null,
            organization_id: "o-9",
          });
        }
        if (url.endsWith("/organizations/o-9")) {
          return jsonResponse({ id: "o-9", display_name: "Brandt Automotive" });
        }
        return jsonResponse({ data: [], page: { next_cursor: null } });
      }),
    );
    render(<CommandPalette open onClose={() => {}} commands={commands} />);
    await userEvent.type(screen.getByRole("searchbox"), "roll");
    expect(await screen.findByText("ACME-CRM")).toBeTruthy();
    expect(await screen.findByText("Brandt Automotive")).toBeTruthy();
    expect(screen.queryByText("project")).toBeNull();

    await userEvent.click(screen.getByText("ACME-CRM"));
    expect(window.location.hash).toBe("#/projects/pr-1");
  });

  // Typing a word and being taken to what carries it is the whole point of a
  // tag. The palette used to drop tag hits with the types that have no page —
  // a tag has one, so a searcher was left with no autocomplete for the
  // vocabulary at all.
  it("offers a tag hit and opens its page", async () => {
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
    await userEvent.type(screen.getByRole("searchbox"), "key");
    await waitFor(() => expect(screen.getByText("Key Account")).toBeTruthy());

    await userEvent.click(screen.getByText("Key Account"));
    expect(window.location.hash).toBe("#/tags/t-1");
  });
});

// The builtin set is DERIVED from the rail's destinations, so a screen with a
// rail row is a ⌘K command with no registration of its own — and a screen with
// neither is reachable only by typing its hash. That derivation is what these
// assert, end to end: the word a reader types, and the address they land on.
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

  // The company page rides a deployment flag as well as a grant, and the
  // palette used to answer that half of the question differently from the rail:
  // it passed `probeCompanyFlag: false` to avoid a network read, so the flag
  // resolved to false here and to its real value there. One installation, two
  // answers, and no test could see it because each surface was asserted alone.
  //
  // These three hold the claim that they now agree. The knob is the same
  // `meFixture` field `settings-nav.test.tsx` drives, so a predicate that
  // stopped reading it fails on both sides at once.
  function renderProbeWithCompany(opts: { companyContext: boolean | null }) {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(
          meFixture({
            roles: [],
            allow: { organization: ["read"] },
            settingsAvailability:
              opts.companyContext === null
                ? null
                : { company_context: opts.companyContext },
          }),
        ),
      ),
    );
    return render(<Probe />);
  }

  it("offers the company shortcut when the installation has that surface", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({ companyContext: true });
    await user.type(screen.getByRole("searchbox"), "general");
    await waitFor(() => {
      expect(screen.getAllByRole("button")[0].textContent).toContain("General");
    });
  });

  it("withholds it when the installation does not, matching the rail", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({ companyContext: false });
    // The grant is held and the flag is not, which is exactly the state the old
    // palette got wrong: it never read the flag, so it fell back to the grants
    // beside it and offered a page this installation may not have.
    await user.type(screen.getByRole("searchbox"), "general");
    await waitFor(() => {
      expect(screen.queryByText("General")).toBeNull();
    });
  });

  it("withholds it when /me carries no availability at all", async () => {
    const user = userEvent.setup();
    renderProbeWithCompany({ companyContext: null });
    await user.type(screen.getByRole("searchbox"), "general");
    await waitFor(() => {
      expect(screen.queryByText("General")).toBeNull();
    });
  });

  it("reaches the filter builder by the name the screen prints", async () => {
    const user = userEvent.setup();
    renderProbe();
    // Its own title, not a fourth spelling of it: "views" appears in no other
    // command, so matching on it proves the row carries the screen's own words.
    await user.type(screen.getByRole("searchbox"), "views");
    const rows = screen.getAllByRole("button");
    expect(rows[0].textContent).toContain("Filters & views");
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
    const rows = screen.getAllByRole("button");
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
