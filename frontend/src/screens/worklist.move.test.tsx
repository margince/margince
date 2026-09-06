// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The step the server already decided, performed from the row.
//
// The backend computes one next move per deal, from records, deterministically,
// and caches it per reader. Until now nothing on this page read it: the row
// named a problem and the answer travelled in the response unrendered, so a rep
// was told a deal had gone quiet and left to work out what to do about it.

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  stub,
} from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const TASK_BODY = { subject: "Ask about the renewal", link_type: "deal" };
const ACTIVITY = "01a05500-0000-7000-8000-0000000000e9";

function aQuietDeal(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000d1",
    source: "deal_at_risk",
    category: "deals_at_risk",
    title: "Turbinenbau — renewal",
    band: "keep_momentum",
    destination: "today",
    actions: ["open"],
    subject: { type: "deal", id: "01a05500-0000-7000-8000-0000000000d2" },
    move: { action: "create_task", arguments: TASK_BODY },
    ...over,
  });
}

function aDayWith(item: ReturnType<typeof aQuietDeal>) {
  return day({
    queue: [item],
    summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
  });
}

describe("a deal row carrying a decided next step", () => {
  it("offers the step, rather than naming the problem alone", async () => {
    stub(aDayWith(aQuietDeal()));
    renderWorklist();

    await screen.findByText(/Turbinenbau/);
    expect(
      await screen.findByRole("button", { name: /add this task/i }),
    ).not.toBeNull();
  });

  it("sends the server's own arguments, not a body the row rebuilt", async () => {
    // The move's arguments ARE the task the server prepared. A row that
    // re-derived them from what it happens to render would send a different
    // task than the one the reason on screen describes.
    const sent: unknown[] = [];
    stub(aDayWith(aQuietDeal()));
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/tasks")) {
          const body =
            input instanceof Request
              ? await input.clone().text()
              : String(init?.body ?? "");
          sent.push(JSON.parse(body));
          return jsonResponse({ id: "01a05500-0000-7000-8000-0000000000d3" });
        }
        return passthrough(input, init);
      }),
    );
    const user = userEvent.setup();
    renderWorklist();

    await user.click(
      await screen.findByRole("button", { name: /add this task/i }),
    );
    await waitFor(() => expect(sent).toEqual([TASK_BODY]));
  });

  it("leaves every other move to the link that already draws it", async () => {
    // Only create_task is drawn here. The other moves reach the reader as
    // anchors through moveHref — draft_reply and draft_email open the composer,
    // open_task and open_meeting_brief open what they name — so drawing a
    // button for them too would put two controls for one step on one row, and
    // for open_task two controls that go to different places.
    for (const action of [
      "draft_reply",
      "draft_email",
      "open_task",
      "open_meeting_brief",
    ]) {
      cleanup();
      stub(
        aDayWith(
          aQuietDeal({
            move: { action, arguments: { activity_id: ACTIVITY } },
          }),
        ),
      );
      renderWorklist();

      await screen.findByText(/Turbinenbau/);
      // Each verb's OWN control, not create_task's: open_task draws "Open
      // existing task" and open_meeting_brief "Open the meeting brief", so
      // asserting on the create_task label alone would pass over exactly the
      // duplicate this test is for.
      for (const label of [
        /add this task/i,
        /open existing task/i,
        /open the meeting brief/i,
      ]) {
        expect(
          screen.queryByRole("button", { name: label }),
          `${action} drew a button beside the link that already draws it`,
        ).toBeNull();
      }
    }
  });

  it("does not take a brief item's own verbs away", async () => {
    // A brief item carries a deal subject, and the backend attaches a cached
    // move to any deal-subject row that has none. Drawn before BriefVerbs, the
    // task button would replace Act, Set aside and Dismiss on a row whose own
    // verbs are the whole point of it.
    stub(
      aDayWith(
        aQuietDeal({
          source: "brief_item",
          actions: ["act", "set_aside", "dismiss"],
          move: { action: "create_task", arguments: TASK_BODY },
        }),
      ),
    );
    renderWorklist();

    await screen.findByText(/Turbinenbau/);
    expect(
      screen.queryByRole("button", { name: /add this task/i }),
      "a brief item lost its own verbs to a move button",
    ).toBeNull();
  });

  it("draws no button for a step it cannot perform", async () => {
    // Two undrawable moves: a create_task with no body would post {} and only
    // be refused, and `draft_email` deliberately has no control here.
    for (const move of [{ action: "create_task" }, { action: "draft_email" }]) {
      cleanup();
      stub(aDayWith(aQuietDeal({ move })));
      renderWorklist();

      await screen.findByText(/Turbinenbau/);
      expect(
        screen.queryByRole("button", { name: /add this task/i }),
        `${move.action} drew a button`,
      ).toBeNull();
    }
  });
});
