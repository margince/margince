/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { formatMoney } from "../../format/format";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { askedToClose } from "./deal360/confirmadvance.testkit";
import { DealsScreen } from "./deals";
import { deal, render, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

// Drops deal d1 on a stage column the way the board's drag does. jsdom carries
// no drag-and-drop, so the drop handler is dispatched directly. On touch the
// reader takes the stepper, which its own tests cover.
function dropOnStage(stageId: string, dealId = "d1") {
  const column = document.querySelector(
    `[data-stage="${stageId}"]`,
  ) as HTMLElement;
  const dataTransfer = { getData: () => dealId, setData: () => {} };
  const dropEvent = new Event("drop", { bubbles: true });
  Object.assign(dropEvent, { dataTransfer });
  column.dispatchEvent(dropEvent);
}

describe("DealsScreen", () => {
  // The card takes a view model rather than the wire row, so every fact it
  // states is copied across in toBoardDeal. A field left out there is absent
  // on every card. A rep sorts a quarter by close date, so its loss hides best.
  it("draws a deal's close date on its board card", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ expected_close_date: "2099-10-14" })]),
    );
    render(<DealsScreen />);
    expect(await screen.findByText("closes 14 Oct 2099")).toBeTruthy();
  });

  it("board↔table swaps views over the same fetched set without a reload", async () => {
    const fetchMock = stubBackend([deal({})]);
    vi.stubGlobal("fetch", fetchMock);
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );
    const dealFetches = () =>
      fetchMock.mock.calls.filter((call) =>
        String(
          call[0] && (call[0] as Request).url
            ? (call[0] as Request).url
            : call[0],
        ).includes("/deals"),
      ).length;
    const before = dealFetches();
    await userEvent.click(screen.getByRole("button", { name: "Table" }));
    expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(); // same set, table view
    expect(dealFetches()).toBe(before); // no reload
  });

  // A view saved on the deals list is a server row, and the tab rail has to
  // read it. Otherwise a saved view is storable through the contract and then
  // invisible.
  it("offers a saved view as a tab beside the standard views", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        savedViews: [
          {
            id: "v1",
            resource: "deals",
            name: "Slipping this quarter",
            query: {
              list: { sort: "-amount_minor", filters: { stalled: "true" } },
            },
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    expect(
      await screen.findByRole("button", { name: "Slipping this quarter" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "All" })).toBeTruthy();
  });

  // Picking the tab has to narrow the list, not just highlight: the saved
  // filters travel to the server or the view is decoration.
  it("applies a saved view's filters to the deals request", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onDealsUrl: (url) => urls.push(url),
        savedViews: [
          {
            id: "v1",
            resource: "deals",
            name: "My stalled deals",
            query: { list: { sort: "", filters: { stalled: "true" } } },
            created_at: "2026-06-01T00:00:00Z",
            updated_at: "2026-06-01T00:00:00Z",
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));
    await user.click(
      await screen.findByRole("button", { name: "My stalled deals" }),
    );

    await waitFor(() =>
      expect(urls.some((url) => url.includes("stalled=true"))).toBe(true),
    );
  });

  // The pipeline picker is the strongest dial on this screen and it lives in
  // its own state, outside the list query. Saved without it, a view would
  // restore against whichever pipeline happened to be showing: a different
  // list of deals under the name the reader chose.
  it("saves the selected pipeline as part of the view", async () => {
    let saved: Record<string, unknown> | undefined;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onCreateView: (body) => {
          saved = body as Record<string, unknown>;
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));
    // The save action appears only once something narrows the list; a sort is
    // the cheapest such narrowing to reach from here.
    await user.click(screen.getByRole("button", { name: "Sort by Value" }));
    await user.click(await screen.findByRole("button", { name: "Save view" }));
    await user.type(
      await screen.findByRole("textbox", { name: "Name" }),
      "Pipeline view",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(saved).toBeTruthy());
    if (!saved) {
      throw new Error("the save never reached the server");
    }
    const list = (saved.query as Record<string, Record<string, unknown>>).list;
    expect((list.filters as Record<string, string>).pipeline_id).toBe("pl");
  });

  // A pipeline is always selected, so carrying it into the query must not make
  // an untouched list look narrowed. Otherwise the save action offers to store
  // the default view, the clutter its own check exists to prevent.
  it("offers no save action until the reader narrows something", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    expect(screen.queryByRole("button", { name: "Save view" })).toBeNull();
  });

  // Owner and archive run as one preview and one change on the server. Each
  // deal carries its own version rather than one copied across the selection.
  it("a bulk owner change previews and runs once, with each row's own version", async () => {
    const bulk: { path: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend(
        [
          deal({ id: "d1", name: "First", version: 3 }),
          deal({ id: "d2", name: "Second", version: 9 }),
        ],
        { onBulk: (path, body) => bulk.push({ path, body }) },
      ),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    await user.click(
      await screen.findByRole("checkbox", { name: "Select First" }),
    );
    await user.click(screen.getByRole("checkbox", { name: "Select Second" }));
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "New owner" }),
      "Me",
    );
    await user.click(screen.getByRole("button", { name: "Assign owner" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      await within(dialog).findByRole("button", { name: "Change owner" }),
    );

    await waitFor(() => expect(bulk).toHaveLength(2));
    expect(bulk.map((call) => call.path)).toEqual([
      "/v1/bulk/preview",
      "/v1/bulk/execute",
    ]);
    expect(bulk[1].body).toMatchObject({
      verb: "reassign_owner",
      owner_id: "u-me",
      items: [
        { id: "d1", version: 3 },
        { id: "d2", version: 9 },
      ],
    });
  });

  // The server treats every advance as a transition. It writes a stage-history
  // row and emits deal.stage_changed without asking whether anything moved. So
  // a row already in the target stage is never sent, or velocity reports read
  // a move that never happened.
  it("moving to a stage skips the rows already in it", async () => {
    const advances: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend(
        [
          deal({ id: "d1", name: "Already there", stage_id: "s2" }),
          deal({ id: "d2", name: "Needs moving", stage_id: "s1" }),
        ],
        { onAdvance: (_body, _ifMatch) => advances.push("advance") },
      ),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    await user.click(
      await screen.findByRole("checkbox", { name: "Select Already there" }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "Select Needs moving" }),
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Move to stage" }),
      "Proposal",
    );
    await user.click(screen.getByRole("button", { name: "Move" }));

    // One write, for the row that moves.
    await waitFor(() => expect(advances.length).toBe(1));
  });

  // A closed deal takes no bulk write: archiving it is done or meaningless.
  // Moving it between open stages would be the silent reopen the record page's
  // stepper already refuses.
  it("offers no checkbox on a closed deal", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([
        deal({ id: "d1", name: "Open one" }),
        deal({ id: "d2", name: "Won one", status: "won", stage_id: "s3" }),
      ]),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await user.click(await screen.findByRole("button", { name: "Table" }));

    expect(
      await screen.findByRole("checkbox", { name: "Select Open one" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("checkbox", { name: "Select Won one" }),
    ).toBeNull();
  });

  // The board draws its columns from the deals it holds. With a single capped
  // read, a busy stage shows a fraction of its cards. Its header counts every
  // matching deal, so it reads "40 deals" above six cards. This prevents that.
  it("the board walks past the first page", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "d1", name: "First page deal" })], {
        nextPage: [deal({ id: "d2", name: "Second page deal" })],
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);

    expect(await screen.findByText("First page deal")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Load more" }));

    // Both pages stand together: the walk adds cards, it does not replace them.
    expect(await screen.findByText("Second page deal")).toBeTruthy();
    expect(screen.getByText("First page deal")).toBeTruthy();
  });

  // The column headers count every matching deal through a separate report
  // query. Its key lives under ["deals"], so the invalidation every deal
  // mutation fires reaches both. Otherwise a moved card's old header still
  // counts it. The test asserts the key relationship, since a request count
  // cannot tell a real invalidation from a routine refetch.
  it("keys the column totals under the deals cache so one invalidation reaches both", async () => {
    const keys: unknown[][] = [];
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <DealsScreen />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    for (const query of client.getQueryCache().getAll()) {
      keys.push(query.queryKey as unknown[]);
    }
    const totals = keys.find((key) => key.includes("by-stage-totals"));
    expect(totals).toBeTruthy();
    // invalidateQueries({queryKey:["deals"]}) matches by prefix, so this is
    // the whole claim: the totals live under it.
    expect(totals?.[0]).toBe("deals");
  });

  // The board's column total must come from the deals-by-stage report over
  // every matching deal, not from summing the (capped) page of cards. The
  // seeded card's own amount×probability gives a different, wrong figure. So
  // this proves the board renders the server's number.
  it("renders the board's column total from the deals-by-stage report, not from the loaded cards", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "a", stage_id: "s1", amount_minor: 1 })], {
        stageTotalsRows: [
          {
            stage_id: "s1",
            currency: "EUR",
            deals: 250,
            base_minor: 9_999_999,
            weighted_base_minor: 1_234_567,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(
        screen.getByText(`≈ ${formatMoney(9_999_999, "EUR", "en")}`),
      ).toBeTruthy(),
    );
    expect(
      screen.getByText(`weighted ${formatMoney(1_234_567, "EUR", "en")}`),
    ).toBeTruthy();
    // The true stage count (250), not "1" (the single loaded card's count).
    expect(screen.getByText("250 deals")).toBeTruthy();
  });

  it.each([
    { name: "the report frame", timezone: UTC_ZONE, day: "01/10/2026" },
    {
      name: "the record clock for older responses",
      timezone: undefined,
      day: "02/10/2026",
    },
  ])("reads the valuation date on $name", async ({ timezone, day }) => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ stage_id: "s1" })], {
        stageTotalsAsOf: "2026-10-01T23:30:00Z",
        stageTotalsTimezone: timezone,
        stageTotalsRows: [
          {
            stage_id: "s1",
            deals: 1,
            base_minor: 10000,
            weighted_base_minor: 4000,
            priced_deals: 1,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    expect(
      await screen.findByText(`Converted · rates as of ${day}`),
    ).toBeTruthy();
  });

  // The report measures every deal the reader may see, which is the set the
  // board draws as cards. So no owner filter is needed for a total.
  it("asks for the stage totals with no owner filter set", async () => {
    let totalsAsked = false;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "a", stage_id: "s1" })], {
        onStageTotalsBody: () => {
          totalsAsked = true;
        },
      }),
    );
    render(<DealsScreen />);
    await waitFor(() => expect(totalsAsked).toBe(true));
  });

  // An owner dial on somebody this reader may not measure is refused by the
  // server. The column says so rather than printing the loaded cards' count as
  // though it were the total.
  it("says the owner is outside what the reader may measure when the totals are refused", async () => {
    window.location.hash = "#/deals?owner_id=u-colleague";
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ id: "a", stage_id: "s1" })], {
        stageTotalsStatus: 403,
      }),
    );
    render(<DealsScreen />);
    // Once per stage column that holds a deal, here only s1. An empty column
    // has no sum to refuse, and a board of empty columns each repeating the
    // sentence reads as a board of errors.
    await waitFor(() =>
      expect(
        screen.getAllByText(
          "Loaded deals only. This owner’s totals are outside what you may measure.",
        ).length,
      ).toBe(1),
    );
  });

  // A tag filter withholds totals too, since the report has no tag field and
  // sending one is a 422. The column must say that.
  it("names the tag filter when a tag is what withheld the total", async () => {
    window.location.hash = "#/deals?owner_id=u-me&tag_id=t1";
    vi.stubGlobal("fetch", stubBackend([deal({ id: "a", stage_id: "s1" })]));
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    // Only the column holding a deal (s1) says so; empty columns stay quiet.
    expect(
      screen.getAllByText(
        "Loaded deals only. No total while a tag filter is on.",
      ).length,
    ).toBe(1);
  });

  it.each([
    {
      name: "archived deals are included",
      query: "include_archived=true",
      reason:
        "Loaded deals only. Totals are unavailable while archived deals are included.",
    },
    {
      name: "one stage filters the board's cards",
      query: "stage_id=s1",
      reason: "Loaded deals only. Totals are unavailable with a stage filter.",
    },
  ])("withholds report totals when $name", async ({ query, reason }) => {
    window.location.hash = `#/deals?${query}`;
    const dealUrls: string[] = [];
    let totalsAsked = false;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onDealsUrl: (url) => dealUrls.push(url),
        onStageTotalsBody: () => {
          totalsAsked = true;
        },
        stageTotalsRows: [
          { stage_id: "s1", deals: 250, base_minor: 9_999_999 },
          { stage_id: "s2", deals: 250, base_minor: 9_999_999 },
        ],
      }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    expect(dealUrls.some((url) => url.includes(query))).toBe(true);
    expect(totalsAsked).toBe(false);
    expect(screen.getByText(reason)).toBeTruthy();
    expect(screen.queryByText("250 deals")).toBeNull();
    expect(
      screen.queryByText(`≈ ${formatMoney(9_999_999, "EUR", "en")}`),
    ).toBeNull();
  });

  it("sends the board's active filters to the deals-by-stage totals request", async () => {
    window.location.hash = "#/deals?forecast_category=commit";
    let sentBody: unknown;
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onStageTotalsBody: (body) => {
          sentBody = body;
        },
      }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );
    expect(sentBody).toMatchObject({
      filters: { forecast_category: "commit" },
      group_by: ["stage_id"],
      aggregates: [
        { fn: "count", as: "deals" },
        { fn: "sum", field: "amount_base_minor", as: "base_minor" },
        { fn: "sum", field: "weighted_base_minor", as: "weighted_base_minor" },
        { fn: "count", field: "amount_base_minor", as: "priced_deals" },
      ],
    });
  });

  it("a terminal-stage advance opens the 🟡 confirm and posts only after confirming", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    // simulate the drop on the Won column via the drop handler path
    const wonColumn = document.querySelector(
      '[data-stage="s3"]',
    ) as HTMLElement;
    const dataTransfer = { getData: () => "d1", setData: () => {} };
    const dropEvent = new Event("drop", { bubbles: true }) as unknown as {
      dataTransfer: typeof dataTransfer;
    };
    Object.assign(dropEvent, { dataTransfer });
    wonColumn.dispatchEvent(dropEvent as unknown as Event);

    await askedToClose("Won");
    expect(advances).toHaveLength(0); // nothing posted yet: confirm-first
    await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s3", status: "won" }, "4"]);
  });

  // A deal won on a purchase order or a phone call has no contract to point at.
  // The dialog reads that when it opens and asks how it was won before Confirm,
  // rather than refusing a press that never had the answer.
  it("a deal with no contract asks how it was won before Confirm, and sends the answer", async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");

    // Asked on opening, and Confirm waits for the answer. Nothing is posted yet.
    await screen.findByText("How was it won?");
    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(true);
    expect(advances).toHaveLength(0);

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "On a purchase order",
    );
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "purchase_order",
    });
  });

  it("a deal with a contract is never asked how it was won", async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (body) => advances.push(body) }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(screen.queryByText("How was it won?")).toBeNull();
    expect(advances[0]).toEqual({ to_stage_id: "s3", status: "won" });
  });

  // The server decides, and asks again when the win lands. A contract archived
  // while the dialog sat open leaves the read saying "signed". The first
  // Confirm is refused, and the dialog grows the question then.
  it("a win the server refuses after the read said signed asks how it was won", async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        winEvidenceRead: "signed",
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    expect(screen.queryByText("How was it won?")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    // The dialog stays open rather than dropping the reader back on a board
    // that has snapped the card home.
    await screen.findByText("How was it won?");
    expect(screen.getByText("Move to Won?")).toBeTruthy();
    expect(advances).toHaveLength(1);
    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(true);

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "On a purchase order",
    );
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances).toHaveLength(2));
    expect(advances[1]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "purchase_order",
    });
  });

  it("a contract read that fails leaves the question to the server's refusal", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        winEvidenceRead: "unreadable",
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await screen.findByText("How was it won?");
  });

  // "Other" is the one member that explains nothing on its own, so the
  // server demands a detail after it. Sending the reason without one would be a
  // refusal the reader could have been spared.
  it('picking "Something else" holds Confirm until the detail says what it was', async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    await screen.findByText("How was it won?");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "Other",
    );

    const confirm = () => screen.getByRole("button", { name: "Confirm" });
    expect(confirm().hasAttribute("disabled")).toBe(true);

    await user.type(screen.getByLabelText("Details"), "a barter deal");
    expect(confirm().hasAttribute("disabled")).toBe(false);
    await user.click(confirm());

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "other",
      won_without_contract_detail: "a barter deal",
    });
  });

  // The refusal belongs to the deal that earned it. Read off the shared
  // mutation's error, it outlives that deal. Cancel, open Won on a deal that
  // has a contract, and the reason picker greets it. The server takes a stated
  // reason at its word, so that deal is recorded as won-without-paper.
  it("a refusal on one deal does not ask the next deal how it was won", async () => {
    const withoutPaper = deal({ id: "d1", name: "No contract" });
    const withPaper = deal({ id: "d2", name: "Has contract" });
    vi.stubGlobal(
      "fetch",
      stubBackend([withoutPaper, withPaper], {
        // Only d1 lacks evidence. d2 wins on the first confirm. The read says
        // signed for both, so d1's question comes from the refusal alone.
        demandsWinEvidence: ["d1"],
        winEvidenceRead: "signed",
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() => expect(screen.getByText("No contract")).toBeTruthy());

    dropOnStage("s3");
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await screen.findByText("How was it won?");

    // Walk away from that deal without answering.
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByText("Move to Won?")).toBeNull());

    dropOnStage("s3", "d2");
    await askedToClose("Won");
    expect(screen.queryByText("How was it won?")).toBeNull();
  });

  // A win that succeeds on the first confirm has nothing left to ask, so the
  // dialog must close. Left open, its Confirm stays live over a version the
  // write just replaced.
  it("a win that needs no reason closes the dialog on success", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(screen.queryByText("Move to Won?")).toBeNull());
  });

  // The detail belongs to "Other" alone. The server writes both columns as
  // given, so a detail carried across a change of reason is stored anyway. It
  // leaves text on the deal behind a field the reader can no longer see.
  it('changing away from "Something else" does not send the detail', async () => {
    const advances: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        demandsWinEvidence: true,
        onAdvance: (body) => advances.push(body),
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    await screen.findByText("How was it won?");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "Other",
    );
    await user.type(screen.getByLabelText("Details"), "a barter deal");

    // Change your mind: the detail field disappears, and so must its text.
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "On a purchase order",
    );
    expect(screen.queryByLabelText("Details")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual({
      to_stage_id: "s3",
      status: "won",
      won_without_contract_reason: "purchase_order",
    });
  });

  // The server rejects a detail of format-only characters (`saysSomething` in
  // win_evidence.go). Enabling Confirm on one earns a second refusal for the
  // omission the reader was already asked about.
  it('a zero-width detail does not satisfy "Something else"', async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { demandsWinEvidence: true }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    dropOnStage("s3");
    await askedToClose("Won");
    await screen.findByText("How was it won?");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "How was it won?" }),
      "Other",
    );
    await user.type(screen.getByLabelText("Details"), "​​");

    expect(
      screen.getByRole("button", { name: "Confirm" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("the advance-confirm dot reads the live catalog tier, not a hardcode", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        agentTools: [
          {
            name: "progress_deal",
            title: "Progress a deal with a note",
            description:
              'Move a deal to a new stage and leave a note on its timeline saying why. (Governance: some calls run immediately and others a human approves first, decided per call from its arguments; requires passport scope "write".)',
            required_scope: "write",
            tier: "auto_execute",
            egress: false,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    const wonColumn = document.querySelector(
      '[data-stage="s3"]',
    ) as HTMLElement;
    const dataTransfer = { getData: () => "d1", setData: () => {} };
    const dropEvent = new Event("drop", { bubbles: true }) as unknown as {
      dataTransfer: typeof dataTransfer;
    };
    Object.assign(dropEvent, { dataTransfer });
    wonColumn.dispatchEvent(dropEvent as unknown as Event);

    await askedToClose("Won");
    // progress_deal is catalogued "auto_execute". A hardcoded "confirm" dot
    // would render "Approval first" here instead.
    await waitFor(() =>
      expect(screen.getByLabelText("Automatic")).toBeTruthy(),
    );
  });

  it("an open-stage drop advances without a confirm", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );

    const proposalColumn = document.querySelector(
      '[data-stage="s2"]',
    ) as HTMLElement;
    const dropEvent = new Event("drop", { bubbles: true });
    Object.assign(dropEvent, {
      dataTransfer: { getData: () => "d1", setData: () => {} },
    });
    proposalColumn.dispatchEvent(dropEvent);

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s2" }, "4"]);
    await waitFor(() =>
      expect(screen.getByText("Moved to Proposal")).toBeTruthy(),
    );
  });
});
