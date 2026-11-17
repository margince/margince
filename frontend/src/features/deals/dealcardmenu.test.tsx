/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { stubClipboard } from "../../design-system/clipboard-testing";
import { askedToClose } from "./deal360/confirmadvance.testkit";
import { DealsScreen } from "./deals";
import {
  deal,
  dropOnStage,
  jsonResponse,
  render,
  stubBackend,
} from "./deals.testkit";

// A card's menu moves a deal as a drop does: pinned, and confirmed when terminal.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.history.replaceState(null, "", "/");
  localStorage.clear();
});

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

// Every read and write the board started has answered.
async function settled(client: QueryClient) {
  await waitFor(() =>
    expect(client.isFetching() + client.isMutating()).toBe(0),
  );
}

async function openCardMenu(
  user: ReturnType<typeof userEvent.setup>,
  dealName = "Fleet retrofit",
) {
  const trigger = await screen.findByRole("button", {
    name: `More actions for ${dealName}`,
  });
  await user.click(trigger);
  const panel = document.getElementById(
    trigger.getAttribute("aria-controls") ?? "",
  );
  if (!panel) throw new Error("the card's menu drew no panel");
  return within(panel);
}

// The deal's link, once the board draws it in that stage's column.
function cardLinkIn(stageId: string) {
  return waitFor(() => {
    const column = document.querySelector<HTMLElement>(
      `[data-stage="${stageId}"]`,
    );
    if (!column) throw new Error(`no column for stage ${stageId}`);
    return within(column).getByRole("link", { name: "Fleet retrofit" });
  });
}

const COPY_REFUSED =
  "Could not copy the link. Open the deal and copy the address from the browser’s address bar.";

async function closeShownToast(user: ReturnType<typeof userEvent.setup>) {
  const close = document.querySelector<HTMLElement>(
    '[data-toast-control="close"]',
  );
  if (!close) throw new Error("no toast is showing a close button");
  await user.click(close);
}

describe("deal card menu on the deals board", () => {
  it("moves a deal to the next stage with the version it was drawn from", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(
      await menu.findByRole("button", { name: "Move to Proposal" }),
    );

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s2" }, "4"]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByText("Moved to Proposal")).toBeTruthy();
  });

  // The card is drawn anew in its new column; a keyboard reader stays on it.
  it("keeps focus on the deal it moved", async () => {
    const rows = [deal({})];
    vi.stubGlobal(
      "fetch",
      stubBackend(rows, {
        onAdvance: () => {
          rows[0] = deal({ stage_id: "s2", version: 5 });
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    (await menu.findByRole("button", { name: "Move to Proposal" })).focus();
    await user.keyboard("{Enter}");

    const proposal = await cardLinkIn("s2");
    await waitFor(() => expect(document.activeElement).toBe(proposal));
  });

  it("does not follow a deal whose terminal move was cancelled", async () => {
    const rows = [deal({})];
    vi.stubGlobal(
      "fetch",
      stubBackend(rows, {
        onAdvance: () => {
          rows[0] = deal({ stage_id: "s3", status: "won", version: 5 });
        },
      }),
    );
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    await user.click(await menu.findByRole("button", { name: "Mark won" }));
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    dropOnStage("s3");
    await askedToClose("Won");
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await cardLinkIn("s3");
    await settled(client);
    expect(document.activeElement).not.toBe(await cardLinkIn("s3"));
  });

  it("does not follow a deal whose move was refused", async () => {
    const rows = [deal({})];
    const backend = stubBackend(rows, {
      onAdvance: () => {
        rows[0] = deal({ stage_id: "s2", version: 5 });
      },
    });
    let refused = false;
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (!refused && url.includes("/advance")) {
        refused = true;
        return Promise.resolve(
          jsonResponse({ status: 409, title: "Conflict" }, 409),
        );
      }
      return backend(input, init);
    });
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    await user.click(
      await menu.findByRole("button", { name: "Move to Proposal" }),
    );
    await waitFor(() => expect(refused).toBe(true));
    await settled(client);

    dropOnStage("s2");
    await cardLinkIn("s2");
    await settled(client);
    expect(document.activeElement).not.toBe(await cardLinkIn("s2"));
  });

  it("forgets a saved move once the board's reads leave the card undrawn", async () => {
    const rows = [deal({})];
    vi.stubGlobal(
      "fetch",
      stubBackend(rows, {
        onAdvance: () => {
          rows[0] = deal({ stage_id: "s-hidden", version: 5 });
        },
      }),
    );
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    await user.click(
      await menu.findByRole("button", { name: "Move to Proposal" }),
    );
    await waitFor(() =>
      expect(document.querySelector('[data-deal="d1"]')).toBeNull(),
    );
    await settled(client);

    rows[0] = deal({ stage_id: "s2", version: 5 });
    await client.invalidateQueries({ queryKey: ["deals"] });
    await cardLinkIn("s2");
    await settled(client);
    expect(document.activeElement).not.toBe(await cardLinkIn("s2"));
  });

  it("takes no drag out of the stage list for the card's own", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Move to stage" }));
    const search = await screen.findByRole("combobox", {
      name: "Pick a stage",
    });
    await user.type(search, "Prop");
    const setData = vi.fn();
    fireEvent.dragStart(search, { dataTransfer: { setData } });

    dropOnStage("s2", "Prop");
    expect(setData).not.toHaveBeenCalled();
    expect(client.isMutating()).toBe(0);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // A chat preview reads the path and the page opens the fragment: both name
  // this deal, whatever record page the board was reached from.
  it("copies the deal's own record address", async () => {
    const id = "0b8f6c1e-2a3b-4c5d-8e9f-0a1b2c3d4e5f";
    const other = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
    window.history.replaceState(null, "", `/r/deals/${other}#/deals`);
    vi.stubGlobal("fetch", stubBackend([deal({ id })]));
    const user = userEvent.setup();
    const clipboard = stubClipboard("accepts");
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Copy link" }));

    await waitFor(() =>
      expect(clipboard.written).toEqual([
        `${window.location.origin}/r/deals/${id}#/deals/${id}`,
      ]),
    );
    expect(await screen.findByText("Link copied")).toBeTruthy();
  });

  // The region draws one toast at a time, so a second refusal queued behind
  // the first shows only once the first is closed.
  it("leaves one refused copy to close, however often it is pressed", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    stubClipboard("refuses");
    render(<DealsScreen />);
    const first = await openCardMenu(user);
    await user.click(first.getByRole("button", { name: "Copy link" }));
    await screen.findByText(COPY_REFUSED);
    const second = await openCardMenu(user);
    await user.click(second.getByRole("button", { name: "Copy link" }));
    await screen.findByText(COPY_REFUSED);

    await closeShownToast(user);
    expect(screen.queryByText(COPY_REFUSED)).toBeNull();
  });

  it("leaves one refused copy to close across two cards", async () => {
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({}), deal({ id: "d2", name: "Harbour cranes" })]),
    );
    const user = userEvent.setup();
    stubClipboard("refuses");
    render(<DealsScreen />);
    const first = await openCardMenu(user);
    await user.click(first.getByRole("button", { name: "Copy link" }));
    await screen.findByText(COPY_REFUSED);
    const second = await openCardMenu(user, "Harbour cranes");
    await user.click(second.getByRole("button", { name: "Copy link" }));
    await screen.findByText(COPY_REFUSED);

    await closeShownToast(user);
    expect(screen.queryByText(COPY_REFUSED)).toBeNull();
  });

  it("asks before marking a deal won, and posts only after the confirm", async () => {
    const advances: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onAdvance: (b, m) => advances.push([b, m]) }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(await menu.findByRole("button", { name: "Mark won" }));

    await askedToClose("Won");
    expect(advances).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual([{ to_stage_id: "s3", status: "won" }, "4"]);
  });

  it("closes the menu behind the confirm when Won is picked from the stage list", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Move to stage" }));
    await user.click(await screen.findByRole("option", { name: "Won" }));

    await askedToClose("Won");
    const trigger = screen.getByRole("button", {
      name: "More actions for Fleet retrofit",
    });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });
});

// The board's backend, with a colleague to hand a deal to and a PATCH the
// test may refuse.
function withColleague(
  backend: ReturnType<typeof stubBackend>,
  refusal?: Record<string, unknown>,
) {
  return (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = request ? request.url : String(input);
    const method = request?.method ?? init?.method ?? "GET";
    if (method === "GET" && url.includes("/users")) {
      return Promise.resolve(
        jsonResponse({
          data: [
            { id: "u-me", display_name: "Me", email: "me@acme.test" },
            {
              id: "u-42",
              display_name: "Dana Fischer",
              email: "dana@acme.test",
            },
          ],
          page: { next_cursor: null },
        }),
      );
    }
    if (refusal && method === "PATCH") {
      return backend(input, init).then(() =>
        jsonResponse(refusal, Number(refusal.status)),
      );
    }
    return backend(input, init);
  };
}

describe("changing a deal's owner from its card", () => {
  it("sits right after the close date, which follows the moves", async () => {
    vi.stubGlobal("fetch", withColleague(stubBackend([deal({})])));
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await menu.findByRole("button", { name: "Mark won" });

    const names = menu
      .getAllByRole("button")
      .map((button) => button.textContent);
    const moves = names.indexOf("Mark won");
    expect(names.slice(moves + 1, moves + 3)).toEqual([
      "Change close date",
      "Change owner",
    ]);
  });

  it("PATCHes owner_id once with the drawn version, says so and closes the menu", async () => {
    const patches: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      withColleague(
        stubBackend([deal({ owner_id: "u-me" })], {
          onPatch: (body, ifMatch) => patches.push([body, ifMatch]),
        }),
      ),
    );
    const user = userEvent.setup();
    const client = newClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Change owner" }));
    await user.click(
      await screen.findByRole("option", { name: "Dana Fischer" }),
    );

    expect(await screen.findByText("Assigned to Dana Fischer")).toBeTruthy();
    expect(patches).toEqual([[{ owner_id: "u-42" }, "4"]]);
    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["deals"] }),
    );
    expect(
      screen.getByRole("button", { name: "More actions for Fleet retrofit" }),
    ).toHaveAttribute("aria-expanded", "false");
  });

  it("writes nothing when the deal's own owner is picked", async () => {
    const patches: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      withColleague(
        stubBackend([deal({ owner_id: "u-42" })], {
          onPatch: (body) => patches.push(body),
        }),
      ),
    );
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Change owner" }));
    const current = await screen.findByRole("option", { name: "Dana Fischer" });
    expect(current).toHaveAttribute("aria-selected", "true");
    await user.click(current);

    await settled(client);
    expect(patches).toEqual([]);
  });

  it("keeps a refused change in its list, saying why", async () => {
    vi.stubGlobal(
      "fetch",
      withColleague(stubBackend([deal({})]), {
        status: 403,
        title: "Forbidden",
        detail: "Dana Fischer may not own deals in this pipeline.",
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await user.click(menu.getByRole("button", { name: "Change owner" }));
    await user.click(
      await screen.findByRole("option", { name: "Dana Fischer" }),
    );

    const list = within(await screen.findByRole("dialog"));
    expect(await list.findByRole("alert")).toHaveTextContent(
      "Dana Fischer may not own deals in this pipeline.",
    );
    expect(screen.queryByText("Assigned to Dana Fischer")).toBeNull();
  });

  it.each([
    ["an archived deal", { archived_at: "2026-06-02T00:00:00Z" }],
    ["a deal the seat may not change", { writable: false }],
  ])("is not offered on %s", async (_, overrides) => {
    vi.stubGlobal("fetch", withColleague(stubBackend([deal(overrides)])));
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await menu.findByRole("button", { name: "Copy link" });

    expect(menu.queryByRole("button", { name: "Change owner" })).toBeNull();
  });
});

async function openCloseDate(user: ReturnType<typeof userEvent.setup>) {
  const menu = await openCardMenu(user);
  await user.click(menu.getByRole("button", { name: "Change close date" }));
  return within(await screen.findByRole("dialog", { name: "Expected close" }));
}

describe("changing a deal's close date from its card", () => {
  it("PATCHes only the close date once, pinned to the drawn version", async () => {
    const patches: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ expected_close_date: "2027-03-14" })], {
        onPatch: (body, ifMatch) => patches.push([body, ifMatch]),
      }),
    );
    const user = userEvent.setup();
    const client = newClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    render(<DealsScreen />, client);
    const dialog = await openCloseDate(user);
    const box = dialog.getByRole("textbox", { name: "Expected close" });
    expect(box).toHaveValue("14/03/2027");
    await user.clear(box);
    await user.type(box, "2027-04-30{Enter}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(patches).toEqual([[{ expected_close_date: "2027-04-30" }, "4"]]);
    expect(invalidate).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["deals"] }),
    );
  });

  it("writes nothing when the date is saved unchanged", async () => {
    const patches: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({ expected_close_date: "2027-03-14" })], {
        onPatch: (body) => patches.push(body),
      }),
    );
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const dialog = await openCloseDate(user);
    await user.click(dialog.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await settled(client);
    expect(patches).toEqual([]);
  });

  it("keeps a refused date in the dialog, saying why", async () => {
    vi.stubGlobal(
      "fetch",
      withColleague(stubBackend([deal({})]), {
        status: 422,
        title: "Unprocessable",
        detail: "An open deal cannot close in the past.",
      }),
    );
    const user = userEvent.setup();
    render(<DealsScreen />);
    const dialog = await openCloseDate(user);
    await user.type(
      dialog.getByRole("textbox", { name: "Expected close" }),
      "2020-01-02{Enter}",
    );

    expect(await dialog.findByRole("alert")).toHaveTextContent(
      "An open deal cannot close in the past.",
    );
    expect(dialog.getByRole("textbox", { name: "Expected close" })).toHaveValue(
      "02/01/2020",
    );
  });

  it("opens from the keyboard and gives focus back to the menu's trigger on Escape", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    menu.getByRole("button", { name: "Change close date" }).focus();
    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", {
      name: "Expected close",
    });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("textbox", { name: "Expected close" }),
      ),
    );

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "More actions for Fleet retrofit" }),
    );
  });

  it.each([
    ["won", { status: "won", stage_id: "s3" }],
    ["lost", { status: "lost", stage_id: "s1" }],
  ] as const)(
    "is not offered on a %s deal, which keeps its owner change",
    async (_, overrides) => {
      vi.stubGlobal("fetch", withColleague(stubBackend([deal(overrides)])));
      const user = userEvent.setup();
      render(<DealsScreen />);
      const menu = await openCardMenu(user);
      await menu.findByRole("button", { name: "Change owner" });

      expect(
        menu.queryByRole("button", { name: "Change close date" }),
      ).toBeNull();
    },
  );

  it("keeps the opened version and the typed day across a refetch of the board", async () => {
    const rows = [deal({ expected_close_date: "2027-03-14" })];
    const patches: [unknown, string | null][] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend(rows, {
        onPatch: (body, ifMatch) => patches.push([body, ifMatch]),
      }),
    );
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const dialog = await openCloseDate(user);
    const box = dialog.getByRole("textbox", { name: "Expected close" });
    await user.clear(box);
    await user.type(box, "2027-04-30{Tab}");

    rows[0] = deal({ expected_close_date: "2027-03-14", version: 5 });
    await client.invalidateQueries({ queryKey: ["deals"] });
    await settled(client);
    expect(box).toHaveValue("30/04/2027");
    await user.click(dialog.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(patches).toEqual([[{ expected_close_date: "2027-04-30" }, "4"]]);
  });

  it("shows itself busy, and opens nothing, while another write on the deal is out", async () => {
    vi.stubGlobal("fetch", stubBackend([deal({})]));
    const user = userEvent.setup();
    const client = newClient();
    render(<DealsScreen />, client);
    const menu = await openCardMenu(user);
    const item = menu.getByRole("button", { name: "Change close date" });
    void client
      .getMutationCache()
      .build(client, {
        mutationKey: ["deal-edit", "d1"],
        mutationFn: () => new Promise<never>(() => undefined),
      })
      .execute(undefined);

    await waitFor(() => expect(item).toHaveAttribute("aria-busy", "true"));
    await user.click(item);
    expect(screen.queryByRole("dialog", { name: "Expected close" })).toBeNull();
  });

  it.each([
    ["an archived deal", { archived_at: "2026-06-02T00:00:00Z" }],
    ["a deal the seat may not change", { writable: false }],
  ])("is not offered on %s", async (_, overrides) => {
    vi.stubGlobal("fetch", stubBackend([deal(overrides)]));
    const user = userEvent.setup();
    render(<DealsScreen />);
    const menu = await openCardMenu(user);
    await menu.findByRole("button", { name: "Copy link" });

    expect(
      menu.queryByRole("button", { name: "Change close date" }),
    ).toBeNull();
  });
});
