/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import { ComposeModal } from "./compose";
import {
  type Activity,
  activity202,
  findThreadChoice,
  jsonResponse,
  pickWhy,
  render,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// What the composer claims a message is, and when it claims nothing.
//
// The rule: a reply to the subject's own message derives reply_to_inbound from
// the anchor, so the composer asks nothing and sends no claim. Everything else
// has to be asked, because there is nothing to derive from.
describe("what the composer says this message is", () => {
  const inbound: Activity = {
    ...activity202,
    thread_key: "<t-why@mail>",
    body: "Can you send the revised quote?",
    direction: "inbound",
  };

  it("asks nothing on a reply, and says why not", async () => {
    stubRoutes({ "GET /activities/act-1": () => jsonResponse(inbound) });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    // A reader who is shown a dropdown here is being asked to restate what
    // the thread in front of them says. The warning bar stays empty.
    await screen.findByText(/Replying to “Re: Q3”/);
    expect(screen.queryByLabelText("Reason for contact")).toBeNull();
  });

  it("sends a reply with no claim at all", async () => {
    const sent = stubRoutes({
      "GET /activities/act-1": () => jsonResponse(inbound),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByText(/Replying to “Re: Q3”/);
    await userEvent.type(screen.getByLabelText("To"), "a@x.com");
    await userEvent.tab();
    await userEvent.type(screen.getByLabelText("Subject"), "Re: quote");
    writeMessage("Body", "Attached.");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.find((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBeTruthy(),
    );
    const body = sent.find((r) => r.key === "POST /activities/act-1/send-email")
      ?.body as Record<string, unknown>;
    // Neither key. The engine resolves reply_to_inbound from the anchor, and a
    // claim on top could only agree with it or contradict it. A contradiction
    // is recorded as a claim the evidence does not carry.
    expect(body).not.toHaveProperty("communication_context");
    expect(body).not.toHaveProperty("consent_purpose");
  });

  it("drops a stale answer when the composer moves onto a reply", async () => {
    // On a reply the question never renders. So a composer that passed its own
    // state through would look correct, as the state is empty when nothing set
    // it. It stops looking correct once a reader answers on an unanchored
    // message and then opens a reply, one click in the same drawer.
    //
    // The thread is the stronger evidence either way, so the stale pick must
    // not travel. The server would record it as a claim the evidence does not
    // carry.
    const sent = stubRoutes({
      "GET /activities/act-1": () => jsonResponse(inbound),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    // A parent that flips the anchor on demand, so the same mounted composer
    // moves from unanchored to answering. A rep does that by clicking Reply
    // beside a message. Remounting instead would reset the state and prove
    // nothing about a stale answer surviving.
    function Harness() {
      const [anchored, setAnchored] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setAnchored(true)}>
            anchor it
          </button>
          <ComposeModal
            activityId={anchored ? "act-1" : undefined}
            entityType="contact"
            entityId="p-1"
            open
            onClose={vi.fn()}
          />
        </>
      );
    }
    render(<Harness />);

    await screen.findByRole("combobox", { name: WHY_ASK });
    await pickWhy(WHY_LABEL.marketing);

    await userEvent.click(screen.getByRole("button", { name: "anchor it" }));
    await screen.findByText(/Replying to “Re: Q3”/);

    await userEvent.type(screen.getByLabelText("To"), "a@x.com");
    await userEvent.tab();
    await userEvent.type(screen.getByLabelText("Subject"), "Re: quote");
    writeMessage("Body", "Attached.");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.find((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBeTruthy(),
    );
    const body = sent.find((r) => r.key === "POST /activities/act-1/send-email")
      ?.body as Record<string, unknown>;
    expect(body).not.toHaveProperty("communication_context");
  });

  it("asks on a first message, and sends what the reader answered", async () => {
    const sent = stubRoutes({
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByRole("combobox", { name: WHY_ASK });
    await userEvent.type(screen.getByLabelText("To"), "a@x.com");
    await userEvent.tab();
    await userEvent.type(screen.getByLabelText("Subject"), "Hello");
    writeMessage("Body", "First contact.");
    await pickWhy(WHY_LABEL.requestedFollowup);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(sent.find((r) => r.key === "POST /emails")).toBeTruthy(),
    );
    const body = sent.find((r) => r.key === "POST /emails")?.body as Record<
      string,
      unknown
    >;
    expect(body.communication_context).toBe("requested_followup");
  });

  it("refuses to send a first message until the reader says why", async () => {
    let sent = false;
    stubRoutes({
      "POST /emails": () => {
        sent = true;
        return jsonResponse(activity202, 202);
      },
    });
    render(
      <ComposeModal
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.type(await screen.findByLabelText("To"), "a@x.com");
    await userEvent.tab();
    await userEvent.type(screen.getByLabelText("Subject"), "Hello");
    writeMessage("Body", "First contact.");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      await screen.findByText("Select the reason for contact."),
    ).toBeTruthy();
    expect(sent).toBe(false);
  });
});

// The conversation a reply is answering: the thread menu above the fields, and
// the earlier messages quoted under the words.
describe("the composer's conversation pane", () => {
  const threadMenu = () => {
    const menu = document.querySelector<HTMLElement>(".compose-threadmenu");
    if (!menu) throw new Error("the thread menu is shut");
    return menu;
  };
  const threaded: Activity = {
    ...activity202,
    thread_key: "<t-1@mail>",
    body: "The signed order is attached.",
    direction: "inbound",
  };
  const sibling: Activity = {
    ...threaded,
    id: "act-0",
    subject: "Q3 order",
    body: "Sending the order over today.",
    direction: "outbound",
    occurred_at: "2026-06-28T00:00:00Z",
  };

  it("marks the thread it is answering in the menu above the fields", async () => {
    stubRoutes({
      "GET /activities/act-1": () => jsonResponse(threaded),
      "GET /activities": () =>
        jsonResponse({ data: [threaded, sibling], page: { has_more: false } }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="company"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The thread the draft answers is marked in the menu the bar opens.
    await userEvent.click(
      await screen.findByRole("button", { name: /Change$/ }),
    );
    expect(
      await within(threadMenu()).findByRole("button", { pressed: true }),
    ).toBeTruthy();
    expect(document.querySelector(".docked .compose-threadbar")).not.toBeNull();
  });

  it("draws a message it may not read as a held place, not a gap", async () => {
    // A thread can run through an audience this reader is not in. Dropping the
    // row would make the conversation look continuous when it is not. The
    // reply would be written into a silence the writer cannot see.
    const withheld: Activity = {
      ...sibling,
      id: "act-2",
      subject: null,
      body: null,
      content_state: "withheld",
    };
    stubRoutes({
      "GET /activities/act-1": () => jsonResponse(threaded),
      "GET /activities": () =>
        jsonResponse({ data: [threaded, withheld], page: { has_more: false } }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="company"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The words the draft answers are quoted under it; the withheld one is
    // not, since nobody here may read it.
    await userEvent.click(
      await screen.findByRole("button", { name: "Show 1 earlier message" }),
    );
    expect(
      await screen.findByText("The signed order is attached."),
    ).toBeTruthy();
    // The menu still holds the withheld message's place in the thread.
    await userEvent.click(screen.getByRole("button", { name: /Change$/ }));
    await waitFor(() =>
      expect(
        threadMenu().querySelectorAll(".compose-threadlist-others > li"),
      ).toHaveLength(1),
    );
  });

  it("anchors nothing until the reader picks a thread", async () => {
    // Nothing is chosen on the reader's behalf: the threads are offered and
    // the reader takes one.
    stubRoutes({
      "GET /activities": () =>
        jsonResponse({ data: [threaded, sibling], page: { has_more: false } }),
      // Choosing a thread reads its newest message, the way a Reply on the
      // History tab does.
      "GET /activities/act-1": () => jsonResponse(threaded),
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(await screen.findByText("New thread")).toBeTruthy();
    const choice = await findThreadChoice(/Re: Q3/);
    expect(choice.getAttribute("aria-pressed")).toBe("false");
    expect(screen.queryByRole("button", { name: "New email" })).toBeNull();

    // Taking one marks it as the thread the draft answers, with a way back.
    await userEvent.click(choice);
    await userEvent.click(
      await screen.findByRole("button", { name: /Change$/ }),
    );
    const menu = within(threadMenu());
    await waitFor(() =>
      expect(
        menu
          .getByRole("button", { name: /Re: Q3/ })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    expect(menu.getByRole("button", { name: "New email" })).toBeTruthy();
  });

  it("draws no thread for a reply to a note", async () => {
    // A timeline row offers Write email on a note as readily as Reply on a
    // mail. A note offered as a thread would claim to be an exchange.
    const note: Activity = { ...threaded, kind: "note" };
    const sent = stubRoutes({
      "GET /activities/act-1": () => jsonResponse(note),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="company"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The composer has read the anchor and the record's threads. So the
    // menu's empty list is a decision, not one that had not arrived yet.
    await waitFor(() =>
      expect(
        sent.filter((call) => call.key === "GET /activities/act-1"),
      ).not.toHaveLength(0),
    );
    await screen.findByText(/^About: /);
    await userEvent.click(screen.getByRole("button", { name: /Change$/ }));
    expect(threadMenu().querySelectorAll(".compose-threadrow")).toHaveLength(0);
  });

  it("shows a mail with no thread as the one message it is", async () => {
    // Capture assigns a thread key only where a provider reported a
    // conversation. A mail without one is a conversation of one message.
    // Asking the server for every activity that has no thread is the wrong
    // question.
    const loose: Activity = { ...threaded, thread_key: null };
    const sent = stubRoutes({
      "GET /activities/act-1": () => jsonResponse(loose),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="company"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Show 1 earlier message" }),
    );
    await waitFor(() =>
      expect(document.querySelector(".compose-earlier")?.textContent).toContain(
        "The signed order is attached.",
      ),
    );
    expect(
      sent.filter(
        (call) =>
          call.key === "GET /activities" && call.query.has("thread_key"),
      ),
    ).toHaveLength(0);
  });

  it("searches a long record's threads and reads older ones a page at a time", async () => {
    const user = userEvent.setup();
    const older: Activity = {
      ...sibling,
      id: "act-9",
      subject: "Invoice 1040",
    };
    const lastYear: Activity = {
      ...sibling,
      id: "act-8",
      subject: "Q2 renewal",
      thread_key: "<t-renewal@mail>",
    };
    // The first read has more behind it; the next is the record's last page.
    let reads = 0;
    const sent = stubRoutes({
      "GET /activities": () => {
        reads += 1;
        return jsonResponse(
          reads === 1
            ? { data: [threaded], page: { has_more: true, next_cursor: "c-2" } }
            : { data: [lastYear], page: { has_more: false } },
        );
      },
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );
    await user.click(
      await screen.findByRole("button", { name: /Continue a thread$/ }),
    );

    // Older mail is a page away, read on asking.
    await user.click(
      await within(threadMenu()).findByRole("button", { name: "Show more" }),
    );
    await waitFor(() =>
      expect(
        sent.some(
          (call) =>
            call.key === "GET /activities" &&
            call.query.get("cursor") === "c-2",
        ),
      ).toBe(true),
    );
    expect(
      await within(threadMenu()).findByRole("button", { name: /Q2 renewal/ }),
    ).toBeTruthy();
    expect(
      within(threadMenu()).queryByRole("button", { name: "Show more" }),
    ).toBeNull();

    // Words typed are asked of the server, not filtered from one page.
    const searched = stubRoutes({
      "GET /activities": () =>
        jsonResponse({ data: [older], page: { has_more: false } }),
    });
    await user.type(within(threadMenu()).getByRole("searchbox"), "invoice");
    expect(
      await within(threadMenu()).findByRole("button", { name: /Invoice 1040/ }),
    ).toBeTruthy();
    expect(
      searched.some(
        (call) =>
          call.key === "GET /activities" && call.query.get("q") === "invoice",
      ),
    ).toBe(true);
  });
});
