/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { formatDayMonth, formatMoney } from "../../format/format";
import { UTC_ZONE } from "../../format/timezone";
import { translate } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { DealScreen } from "./deals";
import {
  type Deal,
  deal,
  jsonResponse,
  type Offer,
  offer,
  REP_GRANTS,
  render,
  stages,
  stubBackend,
} from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

function stubDealBackend(
  onRecord: Deal,
  offers: Offer[],
  onCreateOffer?: (body: unknown) => void,
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (url.includes("/me")) {
      return jsonResponse(meFixture({ allow: REP_GRANTS }));
    }
    if (url.includes("/pipelines")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/context")) {
      return jsonResponse({ anchor: { type: "deal", id: "d1" }, sections: [] });
    }
    if (method === "POST" && url.includes("/offers")) {
      const body = request
        ? await request.json()
        : JSON.parse(String(init?.body));
      onCreateOffer?.(body);
      return jsonResponse(
        offer({ id: "new-offer", currency: body.currency }),
        201,
      );
    }
    if (url.includes("/offers")) {
      return jsonResponse({ data: offers, page: { next_cursor: null } });
    }
    if (url.includes("/history")) {
      return jsonResponse({
        data: [
          {
            id: "h1",
            actor_type: "human",
            actor_id: "u1",
            actor_name: "Demo Admin",
            action: "update",
            occurred_at: "2026-07-13T10:00:00Z",
            summary: "Deal amount changed",
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (url.includes("/stakeholders")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/activities")) {
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }
    if (url.includes("/deals/")) {
      return jsonResponse(onRecord);
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

/**
 * The header's overflow, where every verb but the mail lives. Edit, share,
 * reopen and archive each get a whole line rather than a place in a row.
 * So reaching any of them is two presses: the menu, then the row.
 */
async function openHeaderMenu(user: UserEvent | typeof userEvent = userEvent) {
  await user.click(await screen.findByRole("button", { name: "More actions" }));
}

// The deal's fields are written in the details drawer, behind the overflow.
async function openEditForm(user: UserEvent | typeof userEvent = userEvent) {
  await openHeaderMenu(user);
  await user.click(
    await screen.findByRole("button", {
      name: translate("en", "deal.editDetails"),
    }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Change Deal name" }),
  );
}

describe("DealScreen — edit, archive (A3)", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("edit prefills and PATCHes with If-Match", async () => {
    const patches: { body: unknown; ifMatch: string | null }[] = [];
    const d = deal({ id: "x", version: 4, owner_id: null });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onPatch: (b, h) => patches.push({ body: b, ifMatch: h }),
      }),
    );
    render(<DealScreen id="x" />);
    await openEditForm();
    await userEvent.type(
      screen.getByRole("textbox", { name: "Deal name" }),
      " updated{Enter}",
    );
    await waitFor(() => expect(patches.length).toBe(1));
    expect(patches[0].ifMatch).toBe("4");
  });

  // A deal a partner brought must not look identical to one we won alone.
  // The partner is the fact a commission is computed from.
  it("names the partner that brought the deal, and how, in its details", async () => {
    const d = deal({
      id: "x",
      company_id: "o1",
      partner_company_id: "p1",
      partner_attribution: "sourced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = request.url;
        // EntityRef resolves each reference by its own id read; a reference it
        // cannot name is not a link.
        if (url.includes("/companies/p1")) {
          return jsonResponse({ id: "p1", display_name: "VietnamPartner JSC" });
        }
        return stubBackend([d], { single: d })(request);
      }),
    );

    render(<DealScreen id="x" />);

    // One row names the partner and what it did, so both are read off its
    // text rather than off two elements.
    const partner = await screen.findByText(/VietnamPartner JSC/);
    expect(partner.closest("aside")).not.toBeNull();
    expect(partner.textContent).toContain(
      translate("en", "deal.attributionSourced"),
    );
  });

  // A form offers nothing the record already carries as a blank. The partner
  // picker offers one capped page of partners, so a deal's own partner can be
  // missing from it. A select whose stored value is not an option shows blank.
  // The patch would read that as "Unset" and send a real null, clearing the
  // partner and its commission attribution.
  //
  // The save says nothing about the partner at all, which is what makes it
  // safe: omitted means unchanged.
  it("keeps a partner the picker cannot reach, rather than clearing it on save", async () => {
    const user = userEvent.setup();
    const patches: { body: unknown }[] = [];
    // Neither the company list ("Acme") nor the partner list holds p-offpage.
    const d = deal({
      id: "x",
      version: 4,
      partner_company_id: "p-offpage",
      partner_attribution: "sourced",
    });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onPatch: (body) => patches.push({ body }),
      }),
    );

    render(<DealScreen id="x" />);
    await openEditForm(user);
    await user.type(
      screen.getByRole("textbox", { name: "Deal name" }),
      " updated{Enter}",
    );

    await waitFor(() => expect(patches.length).toBe(1));
    const body = patches[0].body as Record<string, unknown>;
    expect("partner_company_id" in body).toBe(false);
    expect("partner_attribution" in body).toBe(false);
  });

  // Same guarantee when the partner list never arrives at all. A failed or
  // still-pending /partners read must not read as "this deal has no partner".
  it("keeps the partner when the partner list fails outright", async () => {
    const user = userEvent.setup();
    const patches: { body: unknown }[] = [];
    const d = deal({
      id: "x",
      version: 4,
      partner_company_id: "p-offpage",
      partner_attribution: "influenced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/partners")) {
          return jsonResponse({ title: "Boom" }, 500);
        }
        return stubBackend([d], {
          single: d,
          onPatch: (body) => patches.push({ body }),
        })(request);
      }),
    );

    render(<DealScreen id="x" />);
    await openEditForm(user);
    await user.type(
      screen.getByRole("textbox", { name: "Deal name" }),
      " updated{Enter}",
    );

    await waitFor(() => expect(patches.length).toBe(1));
    const body = patches[0].body as Record<string, unknown>;
    expect("partner_company_id" in body).toBe(false);
    expect("partner_attribution" in body).toBe(false);
  });

  // The partner companies' names come from one keyed read. When it fails, the
  // partners are named by id instead, so the form still asks the question.
  it("offers the partner picker when the partner names read fails", async () => {
    const user = userEvent.setup();
    const d = deal({ id: "x", version: 4, partner_company_id: null });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const url = new URL(request.url);
        if (url.pathname.endsWith("/partners")) {
          return jsonResponse({
            data: [{ company_id: "p9" }],
            page: { next_cursor: null },
          });
        }
        if (url.searchParams.get("relationship_type") === "partner") {
          return jsonResponse({ title: "Boom" }, 500);
        }
        if (url.searchParams.getAll("id").includes("p9")) {
          return jsonResponse({
            data: [{ id: "p9", display_name: "Northgate Partners" }],
            page: { next_cursor: null },
          });
        }
        return stubBackend([d], { single: d })(request);
      }),
    );

    render(<DealScreen id="x" />);
    await user.click(await screen.findByRole("button", { name: /more/i }));
    await user.click(
      await screen.findByRole("button", {
        name: translate("en", "deal.editDetails"),
      }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Change via partner" }),
    );
    await user.click(screen.getByLabelText("via partner"));
    expect(
      await screen.findByRole("option", { name: "Northgate Partners" }),
    ).toBeTruthy();
  });

  // Each fact is its own cell: adjacent spans in one text row run together
  // ("€48,000.00Acme Corpvia Northgate") while every text assertion passes.
  // The head carries no facts strip. The facts the edit form does not hold
  // (the stage, how the deal reached Margince and when) are rows of the
  // details beside it.
  it("names the stage, the source and the day it was created in its details", async () => {
    const d = deal({ id: "x", stage_id: "s2" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));

    render(<DealScreen id="x" />);

    const created = await screen.findByText(translate("en", "list.created"));
    const details = created.closest("aside");
    if (!details) {
      throw new Error("the creation day is not in the details");
    }
    expect(
      within(details).getByText(translate("en", "deals.stage")),
    ).toBeTruthy();
    expect(document.querySelector(".record-facts")).toBeNull();
  });

  // Sourced and influenced are paid differently, so the line has to say which.
  it("says a partner only helped when the deal was influenced, not sourced", async () => {
    const d = deal({
      id: "x",
      partner_company_id: "p1",
      partner_attribution: "influenced",
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/companies/p1")) {
          return jsonResponse({ id: "p1", display_name: "Xentral" });
        }
        return stubBackend([d], { single: d })(request);
      }),
    );

    render(<DealScreen id="x" />);

    const partner = await screen.findByText(/Xentral/);
    expect(partner.textContent).toContain(
      translate("en", "deal.attributionInfluenced"),
    );
  });

  it("archive confirms then DELETEs", async () => {
    let deleted = false;
    const d = deal({ id: "x", version: 1 });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onDelete: () => {
          deleted = true;
        },
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);
    await openHeaderMenu(user);
    await user.click(screen.getByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );
    await waitFor(() => expect(deleted).toBe(true));
  });
});

// The stepper closes a deal from its own page. Dragging its card on the board
// is no path on a touch device, where HTML5 drag never fires.
describe("DealScreen — the stage stepper advances the deal", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("moving to an open stage posts the advance pinned to the version shown", async () => {
    const advances: { body: unknown; ifMatch: string | null }[] = [];
    const d = deal({ id: "x", version: 7, stage_id: "s1" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        onAdvance: (body, ifMatch) => advances.push({ body, ifMatch }),
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Proposal" }));

    await waitFor(() => expect(advances.length).toBe(1));
    expect((advances[0].body as { to_stage_id: string }).to_stage_id).toBe(
      "s2",
    );
    // The version the record was drawn from, so a change made elsewhere
    // meanwhile fails loud rather than being erased.
    expect(advances[0].ifMatch).toBe("7");
  });

  // The advance is only half the job: a write whose confirmation is shown to
  // nobody reads like one that did not happen.
  it("confirms the move on the record itself", async () => {
    const d = deal({ id: "x", version: 7, stage_id: "s1" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Proposal" }));

    expect(await screen.findByText("Moved to Proposal")).toBeTruthy();
  });

  // The deal's tags ride in the context rail, beside the seats, the deal room
  // and the mail card. A contact and a company draw theirs in the same column,
  // the details pane every record page draws.
  it("draws the deal's tags in the context rail", async () => {
    const d = deal({ id: "x" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    // The shell's pane around the screen, open. Without it the screen has no
    // pane to draw, and a bare screen reports the card missing whichever column
    // it was written into.
    render(
      <RecordShell>
        <DealScreen id="x" />
      </RecordShell>,
    );

    const tag = await screen.findByText("Renewal");
    expect(tag.closest("aside")).not.toBeNull();
  });

  // A control that can only fail is worse than none: an archived deal is not
  // moved through the pipeline, it is restored first.
  it("offers no move on an archived deal", async () => {
    const d = deal({ id: "x", archived_at: "2026-07-01T00:00:00Z" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    const step = await screen.findByRole("button", { name: "Proposal" });
    expect(step.hasAttribute("disabled")).toBe(true);
  });

  // Reopening is its own action, with a dialog that says the close date and
  // the frozen rate are being cleared. A stepper button that reopened silently
  // would be a second, quieter door to the same write.
  it("offers no move on a closed deal — reopening has its own action", async () => {
    const d = deal({
      id: "x",
      status: "won",
      stage_id: "s3",
      closed_at: "2026-07-01T00:00:00Z",
    });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    const step = await screen.findByRole("button", { name: "Proposal" });
    expect(step.hasAttribute("disabled")).toBe(true);
  });

  // The dialog stays mounted between openings. An abandoned reason would
  // otherwise still sit there the next time a deal is closed, describing a
  // different deal.
  it("a lost reason typed and then cancelled does not come back", async () => {
    const d = deal({ id: "x", version: 2, stage_id: "s1" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [
              ...stages,
              {
                id: "s4",
                pipeline_id: "pl",
                name: "Lost",
                position: 4,
                semantic: "lost",
                win_probability: 0,
              },
            ],
          },
        ],
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Lost" }));
    await user.type(
      screen.getByRole("textbox", { name: "Lost reason" }),
      "typed then abandoned",
    );
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await user.click(await screen.findByRole("button", { name: "Lost" }));
    expect(
      (screen.getByRole("textbox", { name: "Lost reason" }) as HTMLInputElement)
        .value,
    ).toBe("");
  });

  it("the stage the deal is already in is not a control", async () => {
    const d = deal({ id: "x", stage_id: "s1" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    await screen.findByRole("button", { name: "Proposal" });
    expect(screen.queryByRole("button", { name: "Qualify" })).toBeNull();
  });

  // Terminal stages are the 🟡 confirm (AC-deal-6), and a lost deal must say
  // why. The board's drop enforces the same rule, because it is the same
  // dialog.
  it("closing as lost asks for a reason before anything is written", async () => {
    const advances: { body: unknown; ifMatch: string | null }[] = [];
    const d = deal({ id: "x", version: 2, stage_id: "s1" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], {
        single: d,
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages: [
              ...stages,
              {
                id: "s4",
                pipeline_id: "pl",
                name: "Lost",
                position: 4,
                semantic: "lost",
                win_probability: 0,
              },
            ],
          },
        ],
        onAdvance: (body, ifMatch) => advances.push({ body, ifMatch }),
      }),
    );
    const user = userEvent.setup();
    render(<DealScreen id="x" />);

    await user.click(await screen.findByRole("button", { name: "Lost" }));
    // Nothing is written while the dialog stands open.
    expect(advances.length).toBe(0);

    const confirm = screen.getByRole("button", { name: "Confirm" });
    expect(confirm.hasAttribute("disabled")).toBe(true);

    await user.type(
      screen.getByRole("textbox", { name: "Lost reason" }),
      "went with a competitor",
    );
    await user.click(screen.getByRole("button", { name: "Confirm" }));

    await waitFor(() => expect(advances.length).toBe(1));
    const body = advances[0].body as { status: string; lost_reason: string };
    expect(body.status).toBe("lost");
    expect(body.lost_reason).toBe("went with a competitor");
  });
});

describe("DealScreen — an archived deal keeps its verbs, refused", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("shows Edit, Archive, Share and Reopen disabled, each reachable from the one sentence naming the archive", async () => {
    const d = deal({
      id: "x",
      status: "won",
      stage_id: "s3",
      archived_at: "2026-07-13T00:00:00Z",
    });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);

    await openHeaderMenu();
    // Each is awaited, not read in the tick the first arrived in. The note on
    // the same list in companies.header.test.tsx says why.
    const refused = [
      await screen.findByTestId("archive-record"),
      await screen.findByTestId("share-record"),
      await screen.findByTestId("reopen-open"),
    ];
    for (const control of refused) {
      expect(control.hasAttribute("disabled")).toBe(true);
      // The reason has to be reachable from the control. A disabled button
      // cannot be focused and a `title` on it is announced by nobody. So a
      // sentence the control does not point at reaches no reader who needs it.
      const describedBy = control.getAttribute("aria-describedby");
      expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
        "This deal is archived and takes no changes.",
      );
    }
  });
});

// `writable` is what the server's write gate would answer on a mutation. A
// rep holding deal.update on the object is not offered Edit on a colleague's
// deal. Otherwise the 403 arrives only after the form is filled in.
// The upload, the offer and the stage move are refused the same way.
describe("DealScreen — a live deal that is not the viewer's to change", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("refuses Edit, Archive, Share and New offer from the one sentence the page prints", async () => {
    const notMine = deal({
      id: "x",
      owner_id: "u-someone-else",
      writable: false,
    });
    vi.stubGlobal("fetch", stubDealBackend(notMine, []));
    render(<DealScreen id="x" />);

    const sentence =
      "You cannot change this deal. Ask its owner to share it, or an administrator for edit rights.";
    expect(await screen.findByText(sentence)).toBeTruthy();

    // The offer is hung off the deal through the deal's own write gate. So it
    // is refused by the same fact and points at the same sentence.
    const newOffer = await screen.findByRole("button", { name: "New offer" });
    expect(newOffer.hasAttribute("disabled")).toBe(true);
    expect(
      document.getElementById(newOffer.getAttribute("aria-describedby") ?? "")
        ?.textContent,
    ).toBe(sentence);

    // Every verb in the menu describes itself by an id minted in the page's
    // band on the first render. One minted inside the menu would name no
    // element until the menu had been opened.
    await openHeaderMenu();
    for (const testId of ["archive-record", "share-record"]) {
      const control = await screen.findByTestId(testId);
      expect(control.hasAttribute("disabled")).toBe(true);
      expect(
        document.getElementById(control.getAttribute("aria-describedby") ?? "")
          ?.textContent,
      ).toBe(sentence);
    }
  });

  // Absent is not "unknown", it is "no". A server too old to send the field
  // must fail closed, or the guard is only as good as the oldest server.
  it("treats a deal with no writable field as one it may not change", async () => {
    const withoutWritable = deal({ id: "x", owner_id: "u-someone-else" });
    delete (withoutWritable as { writable?: boolean }).writable;
    vi.stubGlobal("fetch", stubDealBackend(withoutWritable, []));
    render(<DealScreen id="x" />);

    expect(await screen.findByText(/You cannot change this deal/)).toBeTruthy();
    await openHeaderMenu();
    expect(
      screen.queryByRole("button", { name: "Change Deal name" }),
    ).toBeNull();
  });
});

describe("DealScreen reopen", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("reopen is shown only for won/lost and advances to an open stage with status open", async () => {
    const moves: [unknown, string | null][] = [];
    const d = deal({ id: "x", status: "won", stage_id: "s3" });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], { single: d, onAdvance: (b, m) => moves.push([b, m]) }),
    );
    render(<DealScreen id="x" />);
    await openHeaderMenu();
    await userEvent.click(screen.getByTestId("reopen-open"));
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(dialog.getByTestId("reopen-stage-s1"));
    await userEvent.click(dialog.getByRole("button", { name: "Reopen" }));
    await waitFor(() => expect(moves.length).toBe(1));
    expect(moves[0]).toEqual([{ to_stage_id: "s1", status: "open" }, "4"]);
  });

  it("correcting the close day of a closed deal posts the picked day under the version it was read at", async () => {
    const posts: [unknown, string | null][] = [];
    const d = deal({
      id: "x",
      status: "won",
      stage_id: "s3",
      closed_at: "2026-08-20T12:00:00Z",
    });
    vi.stubGlobal(
      "fetch",
      stubBackend([d], { single: d, onCloseDay: (b, m) => posts.push([b, m]) }),
    );
    render(<DealScreen id="x" />);
    await openHeaderMenu();
    await userEvent.click(screen.getByTestId("correct-close-day-open"));
    const dialog = within(await screen.findByRole("dialog"));
    const input = dialog.getByTestId("correct-close-day-input");
    expect((input as HTMLInputElement).value).toBe("2026-08-20");
    fireEvent.change(input, { target: { value: "2026-08-12" } });
    await userEvent.click(
      dialog.getByRole("button", { name: "Save close day" }),
    );
    await waitFor(() => expect(posts.length).toBe(1));
    expect(posts[0]).toEqual([{ closed_on: "2026-08-12" }, "4"]);
  });

  it("offers no close-day correction on an open deal", async () => {
    const d = deal({ id: "z", status: "open" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="z" />);
    await openHeaderMenu();
    await screen.findByTestId("archive-record");
    expect(screen.queryByTestId("correct-close-day-open")).toBeNull();
  });

  it("reopen is not offered for an open deal", async () => {
    const d = deal({ id: "y", status: "open" });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="y" />);
    await openHeaderMenu();
    await screen.findByTestId("archive-record");
    expect(screen.queryByTestId("reopen-open")).toBeNull();
  });
});

describe("DealScreen offers panel", () => {
  it("lists each offer as a row: its number and version, when it was written and its net, and its status in words", async () => {
    vi.stubGlobal(
      "fetch",
      stubDealBackend(deal({}), [
        offer({
          id: "o1",
          offer_number: "OFF-0001",
          revision: 1,
          status: "sent",
          gross_minor: 119_000,
          currency: "EUR",
        }),
      ]),
    );
    render(<DealScreen id="d1" />);
    const title = translate("en", "deal.offers.rowTitle", {
      number: "OFF-0001",
      revision: "1",
    });
    expect(await screen.findByText(title)).toBeTruthy();
    expect(
      screen.getByText(
        translate("en", "deal.offers.rowSub", {
          date: formatDayMonth("2026-06-01T00:00:00Z", "en", UTC_ZONE),
          net: formatMoney(100_000, "EUR", "en"),
        }),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(translate("en", "commercial.offer.sent")),
    ).toBeTruthy();
    expect(screen.queryByText("sent")).toBeNull();
  });

  it("creating a new offer posts a draft and navigates to it", async () => {
    const creates: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      stubDealBackend(deal({ currency: "EUR" }), [], (body) =>
        creates.push(body),
      ),
    );
    render(<DealScreen id="d1" />);
    await waitFor(() =>
      expect(screen.getAllByText("Fleet retrofit")[0]).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "New offer" }));
    await waitFor(() => expect(creates).toHaveLength(1));
    expect(creates[0]).toMatchObject({ currency: "EUR", source: "manual" });
    await waitFor(() =>
      expect(window.location.hash).toBe("#/offers/new-offer"),
    );
  });
});

describe("DealScreen — History tab", () => {
  it("shows record changes on the Changes cut of the chronology", async () => {
    vi.stubGlobal("fetch", stubDealBackend(deal({}), []));
    render(<DealScreen id="d1" />);

    await waitFor(() =>
      expect(screen.getByRole("button", { name: /history/i })).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: /history/i }));
    await userEvent.click(screen.getByRole("button", { name: "Changes" }));

    await screen.findByText(
      translate("en", "history.verb.update", { subject: "Demo Admin" }),
    );
  });
});

describe("DealScreen — the header's Email verb", () => {
  it("carries the same Email verb every record header does", async () => {
    const d = deal({ id: "x", version: 4 });
    vi.stubGlobal("fetch", stubBackend([d], { single: d }));
    render(<DealScreen id="x" />);
    const verb = await screen.findByRole("button", { name: "Email" });
    expect(verb.querySelector(".lucide-mail")).toBeTruthy();
    expect(verb.hasAttribute("disabled")).toBe(false);
  });
});
