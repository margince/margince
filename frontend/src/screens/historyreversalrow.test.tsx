/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordHistory } from "./historyentries";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function servingOnePage(entries: readonly unknown[]) {
  return vi.fn(async () =>
    jsonResponse({ data: entries, page: { next_cursor: null } }),
  );
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The timestamp a row shows, WITHOUT naming a zone.
//
// What these rows have to prove is that each half of a pair carries its OWN
// time — not what any particular zone renders. Naming a zone to check that
// would pin one here, which is the thing zone-by-purpose.test.ts forbids a
// screen from doing, and pinning the installation's fallback would agree with
// whatever that value happened to be. The two instants are two minutes apart,
// so in every zone their rendered forms differ; that is the claim, and it is
// checkable without one.
function shownTimeIn(row: Element): string {
  const meta = row.querySelector(".tl-meta")?.textContent ?? "";
  const shown = meta.match(/\d{1,2}:\d{2}/);
  if (!shown) {
    throw new Error(`no timestamp in row meta: ${meta}`);
  }
  return shown[0];
}

const AT_CHANGE = "2026-08-27T14:00:00Z";
const AT_REVERSAL = "2026-08-27T14:02:00Z";

// Sam moves a title, and Tin puts it back.
const samsChange = {
  id: "c",
  actor_type: "human",
  actor_id: "human:u-sam",
  actor_name: "Sam Okafor",
  action: "update",
  occurred_at: AT_CHANGE,
  summary: "Sam Okafor updated the record",
  before: { title: "Head of Ops" },
  after: { title: "Head of Platform" },
  undoable: { undoable: false, reason: "already_undone" },
};

const tinsReversal = {
  id: "r1",
  actor_type: "human",
  actor_id: "human:u-tin",
  actor_name: "Tin Nguyen",
  action: "restore",
  occurred_at: AT_REVERSAL,
  summary: "Tin Nguyen restored the record",
  undid_audit_log_id: "c",
  before: { title: "Head of Platform" },
  after: { title: "Head of Ops" },
  undoable: { undoable: true, reason: null },
};

const restore = { version: 7, onRestored: () => {} };

describe("a reversal and the change it reversed, as one row", () => {
  it("collapses the pair by default, showing neither entry's diff", async () => {
    vi.stubGlobal("fetch", servingOnePage([tinsReversal, samsChange]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);

    expect(
      await screen.findByText("Sam Okafor's change, undone by Tin Nguyen"),
    ).toBeTruthy();
    // The two rows are a disclosure away, not on the surface.
    expect(screen.queryByText("Sam Okafor updated the record")).toBeNull();
    expect(screen.queryByText("Tin Nguyen restored the record")).toBeNull();
    // The value the record moved to and back from is not drawn as a diff: the
    // pair came to nothing, so there is nothing to point an arrow at.
    expect(screen.queryByText("Head of Platform")).toBeNull();
    expect(screen.getByText("net: unchanged")).toBeTruthy();
    expect(screen.getByText("Head of Ops")).toBeTruthy();
  });

  it("carries no action of its own while collapsed", async () => {
    vi.stubGlobal("fetch", servingOnePage([tinsReversal, samsChange]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);
    await screen.findByText("Sam Okafor's change, undone by Tin Nguyen");

    // One control, and it opens the pair. Two changes with opposite intents
    // have no honest single label, so the collapsed face offers no verb.
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(1);
    expect(buttons[0].textContent).toContain("Show both changes");
  });

  it("announces the disclosure and toggles it", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", servingOnePage([tinsReversal, samsChange]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);
    const trigger = await screen.findByRole("button", {
      name: /Show both changes/,
    });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    await user.click(trigger);
    expect(
      screen
        .getByRole("button", { name: /Hide/ })
        .getAttribute("aria-expanded"),
    ).toBe("true");
    await user.click(screen.getByRole("button", { name: /Hide/ }));
    expect(
      screen
        .getByRole("button", { name: /Show both changes/ })
        .getAttribute("aria-expanded"),
    ).toBe("false");
  });

  it("shows both rows whole when opened, each with its own actor and time", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", servingOnePage([tinsReversal, samsChange]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);
    await user.click(
      await screen.findByRole("button", { name: /Show both changes/ }),
    );

    const reversal = screen
      .getByText("Tin Nguyen restored the record")
      .closest("li");
    const reversed = screen
      .getByText("Sam Okafor updated the record")
      .closest("li");
    if (!reversal || !reversed) throw new Error("expected both rows");

    // The attribution chip names the actor inside its own sentence.
    expect(reversal.querySelector(".provenance")?.textContent).toContain(
      "Tin Nguyen",
    );
    expect(reversed.querySelector(".provenance")?.textContent).toContain(
      "Sam Okafor",
    );
    // Two instants two minutes apart: each row shows its own, so the pair is
    // not drawing one timestamp twice.
    expect(shownTimeIn(reversal)).not.toEqual(shownTimeIn(reversed));
    // Each row keeps its own diff, in its own direction.
    expect(within(reversal).getByText("Head of Ops")).toBeTruthy();
    expect(within(reversed).getByText("Head of Platform")).toBeTruthy();
  });

  it("offers Redo on the reversal and refuses the row it reversed", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", servingOnePage([tinsReversal, samsChange]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);
    await user.click(
      await screen.findByRole("button", { name: /Show both changes/ }),
    );

    const reversal = screen
      .getByText("Tin Nguyen restored the record")
      .closest("li");
    if (!reversal) throw new Error("expected the reversal's row");
    // Pressing it re-applies the change: "Put back" would be the same words
    // for the opposite intent, on the row above the one it undid.
    expect(within(reversal).getByRole("button", { name: /Redo/ })).toBeTruthy();
    expect(within(reversal).queryByText("Put back")).toBeNull();
    expect(
      screen.getByText("This change has already been put back."),
    ).toBeTruthy();
  });
});

describe("a pair that only partly went back", () => {
  // One field's clear is refused and the other goes back: the row that would
  // claim "unchanged" over data that moved is the worst outcome available here.
  const partial = {
    ...samsChange,
    before: { title: "Head of Ops", source: "referral" },
    after: { title: "Head of Platform", source: "web" },
  };

  it("says partly undone and shows the residual on its face", async () => {
    vi.stubGlobal("fetch", servingOnePage([tinsReversal, partial]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);

    expect(
      await screen.findByText(
        "Sam Okafor's change, partly undone by Tin Nguyen",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByText("Sam Okafor's change, undone by Tin Nguyen"),
    ).toBeNull();
    expect(screen.queryByText("net: unchanged")).toBeNull();
    expect(screen.getByText("still changed")).toBeTruthy();
    // The field that did NOT go back, as the diff it still is.
    expect(screen.getByText("Source")).toBeTruthy();
    expect(screen.getByText("referral")).toBeTruthy();
    expect(screen.getByText("web")).toBeTruthy();
  });
});

describe("one person correcting themselves", () => {
  it("does not name them twice", async () => {
    const ownReversal = {
      ...tinsReversal,
      actor_id: samsChange.actor_id,
      actor_name: samsChange.actor_name,
      summary: "Sam Okafor restored the record",
    };
    vi.stubGlobal("fetch", servingOnePage([ownReversal, samsChange]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);

    expect(
      await screen.findByText("Sam Okafor undid their own change"),
    ).toBeTruthy();
    expect(
      screen.queryByText("Sam Okafor's change, undone by Sam Okafor"),
    ).toBeNull();
  });
});

describe("a reversal whose partner is not on the page", () => {
  it("stands alone, saying only what it can prove", async () => {
    vi.stubGlobal("fetch", servingOnePage([tinsReversal]));
    render(<RecordHistory kind="person" id="p1" restore={restore} />);

    expect(await screen.findByText("undoing an earlier change")).toBeTruthy();
    expect(screen.getByText("Tin Nguyen restored the record")).toBeTruthy();
    // Never a half-empty pair: nothing to disclose, and its own verb is redo.
    expect(
      screen.queryByRole("button", { name: /Show both changes/ }),
    ).toBeNull();
    expect(screen.getByRole("button", { name: /Redo/ })).toBeTruthy();
  });
});
