// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The step the server already decided, performed from the row.
//
// The backend computes one next move per deal, from records, deterministically,
// and caches it per reader. Until now nothing on this page read it: the row
// named a problem and the answer travelled in the response unrendered, so a rep
// was told a deal had gone quiet and left to work out what to do about it.

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  stub,
} from "../../testing/worklist.testkit";

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
      await screen.findByRole("button", { name: /^add task$/i }),
    ).not.toBeNull();
  });

  it.each(["deal_at_risk", "brief_item"])(
    "%s sends the server's own task arguments",
    async (source) => {
      // Creating the task must preserve the server-prepared arguments.
      const sent: unknown[] = [];
      stub(aDayWith(aQuietDeal({ source })));
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
        await screen.findByRole("button", { name: /^add task$/i }),
      );
      await waitFor(() => expect(sent).toEqual([TASK_BODY]));
    },
  );

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
        /^add task$/i,
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
    expect(screen.getByRole("button", { name: /^add task$/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: en["brief.act"] })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["brief.snooze"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: en["deal.openDeal"] }),
    ).toBeNull();
  });

  it("wears the mark only on a link where the model writes", async () => {
    // A deal's draft move reaches the deal, where nothing drafts on arrival; a
    // contact's opens the composer, which does.
    for (const [subject, marked] of [
      [{ type: "deal", id: "01a05500-0000-7000-8000-0000000000d2" }, false],
      [{ type: "contact", id: "01a05500-0000-7000-8000-0000000000c5" }, true],
    ] as const) {
      cleanup();
      stub(aDayWith(aQuietDeal({ subject, move: { action: "draft_email" } })));
      renderWorklist();

      const link = await screen.findByRole("link", {
        name: marked
          ? en["worklist.verb.draft_email_now"]
          : en["worklist.verb.draft_email"],
      });
      expect(link.classList.contains("btn-ai"), subject.type).toBe(marked);
    }
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
        screen.queryByRole("button", { name: /^add task$/i }),
        `${move.action} drew a button`,
      ).toBeNull();
    }
  });
});

// A WAITING MESSAGE'S PREPARED MOVE IS ITS REPLY, and one control carries both.
//
// The row used to draw two: the reply that opened the composer, and beside it
// the move in the agent's colour — which, depending on what the row was filed
// under, either linked away to the record or opened a READ of the very message
// it offered to draft. Two buttons for one answer, and the one wearing the mark
// was the one that did not draft.
describe("a waiting message whose prepared move is the reply", () => {
  const WAITING = {
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting" as const,
    category: "customer_waiting" as const,
    level: 1,
    title: "Re: pricing for the retrofit",
    actions: ["open", "reply"] as const,
    subject: {
      type: "deal" as const,
      id: "01a05500-0000-7000-8000-00000000bbbb",
      label: "Acme Expansion",
    },
    move: { action: "draft_reply" as const, activity_id: ACTIVITY },
  };

  function aWaitingRow() {
    stub(
      day({
        queue: [row({ ...WAITING, actions: [...WAITING.actions] })],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();
  }

  /** Every control on the row's own line, wearing the agent's mark. */
  function marked(): string[] {
    const acts = document.querySelector(".worklist-row-acts");
    expect(acts, "the row drew no line of verbs").not.toBeNull();
    return [...(acts?.querySelectorAll(".btn-ai") ?? [])].map(
      (verb) => verb.textContent ?? "",
    );
  }

  it("offers one control and not two", async () => {
    aWaitingRow();
    await screen.findByText(/retrofit/);

    // The whole SET of marked controls, not a sample. Asserting only that the
    // draft button exists stays green with the second one still beside it,
    // which is the defect: two buttons wearing the mark, and the other one
    // opened a READING of the message it offered to answer.
    expect(marked()).toEqual([en["worklist.verb.draft_reply_now"]]);
    expect(
      screen.queryByRole("button", { name: en["compose.reply"] }),
      "the row drew a plain reply beside the prepared one",
    ).toBeNull();
  });

  it("keeps the mark on it, because the press drafts", async () => {
    aWaitingRow();

    const draft = await screen.findByRole("button", {
      name: en["worklist.verb.draft_reply_now"],
    });
    expect(draft.classList.contains("btn-ai")).toBe(true);
  });

  it("opens the composer where the reader is standing", async () => {
    // What "does not work" meant: the control wearing the mark navigated or
    // opened a reading. The answer to a waiting message is a composer over the
    // row, and this is the assertion that it is one.
    const user = userEvent.setup();
    aWaitingRow();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.verb.draft_reply_now"],
      }),
    );
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeNull();
    });
  });

  it("starts the draft on that one press", async () => {
    // The mark promises that the model writes. A composer that waited for its
    // own Draft button would make the reader press the same promise twice.
    aWaitingRow();
    const asked: unknown[] = [];
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith(`/activities/${WAITING.id}/draft-email`)) {
          asked.push(
            JSON.parse(
              input instanceof Request
                ? await input.clone().text()
                : String(init?.body ?? "{}"),
            ),
          );
          return jsonResponse({ subject: "Re: pricing", body: "Thanks!" });
        }
        return passthrough(input, init);
      }),
    );
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.verb.draft_reply_now"],
      }),
    );
    await waitFor(() => expect(asked).toHaveLength(1));
  });
});
