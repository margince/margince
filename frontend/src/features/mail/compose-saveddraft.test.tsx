/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import {
  ToastProvider,
  ToastRegion,
  useToast,
} from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { installComposeClock, pressSend } from "./compose.testkit";
import {
  calls,
  composer,
  foundUnderTyping,
  jsonResponse,
  problem,
  type Route,
  render,
  SAVED,
  type Sent,
  stubRoutes,
} from "./compose-saveddraft.testkit";
import { useSavedDraft } from "./composesaveddraft";
import { ComposeWindow } from "./composewindow";

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("a saved draft", () => {
  it("is read for the record the composer opened on and put back in the fields", async () => {
    const sent = stubRoutes({ "GET /mail-drafts": () => jsonResponse(SAVED) });
    render(composer(vi.fn()));

    expect(await screen.findByText("Saved draft restored")).toBeTruthy();
    expect(screen.getByDisplayValue("Pricing for 40 seats")).toBeTruthy();
    await waitFor(() =>
      expect(messageText("Body")).toBe("Half written before lunch"),
    );
    expect(screen.getByText("buyer@acme.test")).toBeTruthy();
    const read = calls(sent, "GET /mail-drafts")[0];
    expect(new URLSearchParams(read?.query).get("anchor_type")).toBe("contact");
    expect(new URLSearchParams(read?.query).get("anchor_id")).toBe("p-1");
  });

  it("keeps an address being typed in To when the saved draft lands", async () => {
    let answer: (response: Response) => void = () => {};
    stubRoutes({
      "GET /mail-drafts": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(composer(vi.fn()));
    const to = await screen.findByRole<HTMLInputElement>("combobox", {
      name: "To",
    });
    await user.type(to, "ada@br");

    act(() => answer(jsonResponse(SAVED)));

    expect(await screen.findByText("Saved draft restored")).toBeTruthy();
    expect(
      screen.getByRole<HTMLInputElement>("combobox", { name: "To" }).value,
    ).toBe("ada@br");
  });

  it("is read for the message a reply answers", async () => {
    const sent = stubRoutes();
    render(composer(vi.fn(), "act-1"));

    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    const query = new URLSearchParams(
      calls(sent, "GET /mail-drafts")[0]?.query,
    );
    expect(query.get("anchor_type")).toBe("activity");
    expect(query.get("anchor_id")).toBe("act-1");
  });

  // A 200 that is not a draft keeps nothing, so the window stays open and says
  // the save failed instead of restoring an empty shape.
  it("stays open when the save answers something that is not a draft", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "PUT /mail-drafts": () => jsonResponse({ data: [], page: {} }),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    writeMessage("Body", "Finished after lunch");

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(await screen.findByText(/The draft was not saved/)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
    expect(messageText("Body")).toBe("Finished after lunch");
  });

  it("saves over the version it restored and closes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "PUT /mail-drafts": () => jsonResponse({ ...SAVED, version: 4 }),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    writeMessage("Body", "Finished after lunch");

    await user.click(screen.getByRole("button", { name: "Close" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const put = calls(sent, "PUT /mail-drafts")[0];
    expect(put?.headers.get("If-Match")).toBe("3");
    expect(put?.body).toEqual({
      anchor_type: "contact",
      anchor_id: "p-1",
      to: ["buyer@acme.test"],
      cc: [],
      bcc: [],
      subject: "Pricing for 40 seats",
      body: "Finished after lunch",
      html_body: "<p>Finished after lunch</p>",
      editor_state: {
        filing: [],
        intent: "",
        ai_generated: false,
        voice_degraded: false,
        served_body: "",
      },
      attachment_ids: [],
    });
    expect(
      await screen.findByText("Draft saved", { selector: ".toast-said" }),
    ).toBeTruthy();
  });

  it("is written when the composer closes over words nobody saved", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "PUT /mail-drafts": () => jsonResponse(SAVED),
    });
    render(composer(onClose));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");

    await user.click(screen.getByRole("button", { name: "Close" }));

    // The close waits for the save, so a refused one cannot drop the text.
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const put = calls(sent, "PUT /mail-drafts")[0];
    expect(put?.headers.get("If-Match")).toBeNull();
    expect(put?.body).toMatchObject({ body: "Half written before lunch" });
  });

  it("is named restored when the composer reopens still holding it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent: Sent[] = stubRoutes({
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
      "PUT /mail-drafts": () => {
        const put = sent
          .filter((call) => call.key === "PUT /mail-drafts")
          .at(-1);
        return jsonResponse({ ...SAVED, ...(put?.body as object), version: 1 });
      },
    });
    function Page() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Write again
          </button>
          <ComposeWindow
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
            open={open}
            onClose={() => setOpen(false)}
          />
        </>
      );
    }
    render(<Page />);
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");
    await user.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await user.click(screen.getByRole("button", { name: "Write again" }));

    expect(await screen.findByText("Saved draft restored")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Delete saved draft" }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(messageText("Body")).toBe("Half written before lunch"),
    );
    // The save's toast stands down, so the delete's confirmation is not
    // queued behind a Delete that has already been offered here. The foot
    // still says the draft is kept; that line is not the toast.
    expect(
      screen.queryByText("Draft saved", { selector: ".toast-said" }),
    ).toBeNull();
    await user.click(
      screen.getByRole("button", { name: "Delete saved draft" }),
    );
    expect(await screen.findByText("Saved draft deleted")).toBeTruthy();
  });

  it("leaves another screen's toast alone when it reopens", async () => {
    // The "Draft saved" toast was already replaced by a later undo; reopening
    // withdraws only its own message, never whatever is on screen now.
    const sent: Sent[] = stubRoutes({
      "PUT /mail-drafts": () => jsonResponse(SAVED),
    });
    function Page() {
      const [open, setOpen] = useState(true);
      const toast = useToast();
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Write again
          </button>
          <button
            type="button"
            onClick={() =>
              toast.show("Task completed", {
                action: { kind: "undo", label: "Undo", onAct: () => {} },
              })
            }
          >
            Complete a task
          </button>
          <ComposeWindow
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
            open={open}
            onClose={() => setOpen(false)}
          />
        </>
      );
    }
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(<Page />);
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(await screen.findByText("Draft saved")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Complete a task" }));
    expect(screen.queryByText("Draft saved")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Write again" }));

    expect(await screen.findByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Task completed")).toBeTruthy();
  });

  it("is not written when the composer closes as it opened", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({ "GET /mail-drafts": () => jsonResponse(SAVED) });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(onClose).toHaveBeenCalled();
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("is not created by opening and closing an empty composer", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes();
    render(composer(onClose));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(onClose).toHaveBeenCalled();
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("can be deleted from the toast the close leaves behind", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "PUT /mail-drafts": () => jsonResponse(SAVED),
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
    });
    render(composer(vi.fn()));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");
    await user.click(screen.getByRole("button", { name: "Close" }));

    await user.click(await screen.findByRole("button", { name: "Delete" }));

    expect(await screen.findByText("Saved draft deleted")).toBeTruthy();
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(1);
    await waitFor(() => expect(messageText("Body")).toBe(""));
  });

  it("is written back by Undo after the trash deletes it, with the same words and recipients", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
      "PUT /mail-drafts": () => jsonResponse({ ...SAVED, id: "md-2" }),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");

    await user.click(screen.getByRole("button", { name: "Delete draft" }));
    expect(await screen.findByText("Saved draft deleted")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() =>
      expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1),
    );
    expect(calls(sent, "PUT /mail-drafts")[0]?.body).toMatchObject({
      anchor_type: "contact",
      anchor_id: "p-1",
      to: ["buyer@acme.test"],
      subject: "Pricing for 40 seats",
      body: "Half written before lunch",
    });
    expect(
      calls(sent, "PUT /mail-drafts")[0]?.headers.get("If-Match"),
    ).toBeNull();
    expect(await screen.findAllByText("Saved draft restored")).not.toHaveLength(
      0,
    );
  });

  describe("Undo after a delete", () => {
    function hookFor(onRestore: (fields: unknown) => void) {
      const client = new QueryClient({
        defaultOptions: { queries: { retry: false } },
      });
      return renderHook(
        (props: { entityId: string; open: boolean }) =>
          useSavedDraft({
            where: {
              answering: undefined,
              entityType: "contact",
              entityId: props.entityId,
              isChannelReply: false,
            },
            open: props.open,
            fields: {
              to: [],
              cc: [],
              bcc: [],
              subject: "",
              body: "",
              html: "",
              files: [],
              context: "",
              sendAt: "",
            },
            replyTo: undefined,
            offeredRecipient: undefined,
            onRestore,
            onClose: () => {},
          }),
        {
          initialProps: { entityId: "p-1", open: true },
          wrapper: ({ children }) => (
            <QueryClientProvider client={client}>
              <LocaleProvider initial="en">
                <ToastProvider>
                  {children}
                  <ToastRegion />
                </ToastProvider>
              </LocaleProvider>
            </QueryClientProvider>
          ),
        },
      );
    }
    const routes = (extra: Record<string, Route> = {}) =>
      stubRoutes({
        "GET /mail-drafts": () => jsonResponse(SAVED),
        "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
        "PUT /mail-drafts": () => jsonResponse({ ...SAVED, id: "md-2" }),
        ...extra,
      });
    const deleteThenUndo = async (
      user: UserEvent,
      hook: ReturnType<typeof hookFor>,
      between: () => void = () => {},
    ) => {
      await waitFor(() => expect(hook.result.current.held).not.toBeNull());
      act(() => hook.result.current.remove());
      await screen.findByText("Saved draft deleted");
      between();
      await user.click(screen.getByRole("button", { name: "Undo" }));
    };

    it("puts the server's answer on screen while the composer is still on that record", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const restored = vi.fn();
      const sent = routes({
        "PUT /mail-drafts": () =>
          jsonResponse({ ...SAVED, id: "md-2", attachments: [] }),
      });
      const hook = hookFor(restored);
      await waitFor(() => expect(hook.result.current.held).not.toBeNull());
      restored.mockClear();
      act(() => hook.result.current.remove());
      await screen.findByText("Saved draft deleted");
      await user.click(screen.getByRole("button", { name: "Undo" }));

      await waitFor(() =>
        expect(restored).toHaveBeenLastCalledWith(
          expect.objectContaining({
            subject: "Pricing for 40 seats",
            files: [],
          }),
        ),
      );
      expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1);
    });

    it("leaves a file the server dropped off the restored words", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const restored = vi.fn();
      routes({
        "GET /mail-drafts": () =>
          jsonResponse({
            ...SAVED,
            attachments: [{ id: "f-1", filename: "a.pdf", byte_size: 10 }],
          }),
        "PUT /mail-drafts": () =>
          jsonResponse({ ...SAVED, id: "md-2", attachments: [] }),
      });
      const hook = hookFor(restored);
      await deleteThenUndo(user, hook);

      await waitFor(() =>
        expect(restored).toHaveBeenLastCalledWith(
          expect.objectContaining({ files: [] }),
        ),
      );
    });

    it("writes the draft back but injects nothing once the composer moved to another record", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const restored = vi.fn();
      let reads = 0;
      const sent = routes({
        "GET /mail-drafts": () =>
          reads++ === 0 ? jsonResponse(SAVED) : problem("not_found", 404),
      });
      const hook = hookFor(restored);
      await deleteThenUndo(user, hook, () => {
        hook.rerender({ entityId: "p-2", open: true });
        restored.mockClear();
      });

      await waitFor(() =>
        expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1),
      );
      expect(calls(sent, "PUT /mail-drafts")[0]?.body).toMatchObject({
        anchor_id: "p-1",
      });
      expect(restored).not.toHaveBeenCalled();
    });

    it("injects nothing into a composer that was closed", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const restored = vi.fn();
      const sent = routes();
      const hook = hookFor(restored);
      await deleteThenUndo(user, hook, () => {
        hook.rerender({ entityId: "p-1", open: false });
        restored.mockClear();
      });

      await waitFor(() =>
        expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1),
      );
      expect(restored).not.toHaveBeenCalled();
    });

    it("does not bring a draft back after a send", async () => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const restored = vi.fn();
      const sent = routes();
      const hook = hookFor(restored);
      await deleteThenUndo(user, hook, () => {
        act(() => hook.result.current.sent());
        restored.mockClear();
      });

      expect(
        await screen.findByText(
          "The message was sent, so its draft stays deleted.",
        ),
      ).toBeTruthy();
      expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
      expect(restored).not.toHaveBeenCalled();
    });
  });

  it("is deleted from the restored notice, and the fields empty", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
    });
    render(composer(vi.fn()));
    await screen.findByText("Saved draft restored");

    await user.click(
      screen.getByRole("button", { name: "Delete saved draft" }),
    );

    await waitFor(() => expect(messageText("Body")).toBe(""));
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(1);
    expect(screen.queryByText("Saved draft restored")).toBeNull();
    expect(screen.queryByDisplayValue("Pricing for 40 seats")).toBeNull();
  });

  it("is never overwritten after another window moved it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    let reads = 0;
    const sent = stubRoutes({
      "GET /mail-drafts": () => {
        reads += 1;
        return jsonResponse(
          reads === 1
            ? SAVED
            : {
                ...SAVED,
                body: "Written in the other tab",
                html_body: "<p>Written in the other tab</p>",
                version: 5,
              },
        );
      },
      "PUT /mail-drafts": () => problem("version_skew", 409),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    writeMessage("Body", "Written here");

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(
      await screen.findByText("Draft changed in another window"),
    ).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
    expect(messageText("Body")).toBe("Written here");
    expect(calls(sent, "GET /mail-drafts")).toHaveLength(2);

    await user.click(
      screen.getByRole("button", { name: "Load saved version" }),
    );
    await waitFor(() =>
      expect(messageText("Body")).toBe("Written in the other tab"),
    );
  });

  it("asks which words win when it lands under words the reader wrote", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = foundUnderTyping(onClose);
    await screen.findByText("You have a saved draft here");
    expect(messageText("Body")).toBe("Written before the read");

    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).not.toHaveBeenCalled();
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);

    await user.click(screen.getByRole("button", { name: "Open saved draft" }));
    await waitFor(() =>
      expect(messageText("Body")).toBe("Half written before lunch"),
    );
  });

  it("is not deleted by discarding words it never held", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = foundUnderTyping(onClose);
    await screen.findByText("You have a saved draft here");

    await user.click(screen.getByRole("button", { name: "Delete draft" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(0);
  });

  it("is replaced only when the reader says so", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = foundUnderTyping(vi.fn());
    await screen.findByText("You have a saved draft here");

    await user.click(
      screen.getByRole("button", { name: "Replace it with this text" }),
    );

    await waitFor(() =>
      expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1),
    );
    const put = calls(sent, "PUT /mail-drafts")[0];
    expect(put?.headers.get("If-Match")).toBe("3");
    expect(put?.body).toMatchObject({ body: "Written before the read" });
  });

  it("is not named to a send that carries other words", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = foundUnderTyping(vi.fn(), {
      "POST /emails": () => jsonResponse({ id: "act-9" }, 202),
    });
    await screen.findByText("You have a saved draft here");
    await user.type(
      screen.getByRole("combobox", { name: "To" }),
      "buyer@acme.test{enter}",
    );
    await user.type(screen.getByLabelText("Subject"), "Pricing");
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Reason for contact" }),
      "They asked for it",
    );

    await pressSend(user);

    await waitFor(() => expect(calls(sent, "POST /emails")).toHaveLength(1));
    expect(calls(sent, "POST /emails")[0]?.body).not.toHaveProperty(
      "mail_draft_id",
    );
  });

  it("keeps the composer open over a failed save once, and a second close discards", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "PUT /mail-drafts": () =>
        new Response(JSON.stringify({ detail: "The draft store is down." }), {
          status: 503,
          headers: { "Content-Type": "application/problem+json" },
        }),
    });
    render(composer(onClose));
    writeMessage("Body", "Half written before lunch");

    await user.click(screen.getByRole("button", { name: "Close" }));

    const line = await screen.findByRole("alert");
    expect(line.firstChild?.textContent).toBe(
      "The draft was not saved. Retry, or close again to discard the text.",
    );
    expect(within(line).getByRole("button", { name: "Retry" })).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
    expect(messageText("Body")).toBe("Half written before lunch");

    await user.click(screen.getByRole("button", { name: "Close" }));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1);
    await waitFor(() => expect(messageText("Body")).toBe(""));
  });

  it("holds the send while a first save is still out", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    let answer: (response: Response) => void = () => {};
    const sent = stubRoutes({
      "PUT /mail-drafts": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
      "POST /emails": () => jsonResponse({ id: "act-9" }, 202),
    });
    render(composer(vi.fn()));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");

    // The quiet save starts a moment after the typing stops.
    await waitFor(
      () => expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1),
      { timeout: 3000 },
    );
    const send = screen.getByRole("button", { name: "Send" });
    expect(send).toHaveProperty("disabled", true);
    await user.click(send);
    expect(calls(sent, "POST /emails")).toHaveLength(0);

    answer(jsonResponse(SAVED));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Send" })).toHaveProperty(
        "disabled",
        false,
      ),
    );
  });

  it("keeps its refused save and Retry in view beside a refused send", async () => {
    stubRoutes({
      "PUT /mail-drafts": () => problem("internal", 503),
      "POST /emails": () => problem("internal", 500),
    });
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    render(composer(vi.fn()));
    await screen.findByRole("combobox", { name: "Reason for contact" });
    writeMessage("Body", "Half written before lunch");
    await user.click(screen.getByRole("button", { name: "Close" }));
    await screen.findByText(/The draft was not saved/);
    await user.type(screen.getByLabelText("Subject"), "Pricing");
    await user.type(
      screen.getByRole("combobox", { name: "To" }),
      "buyer@acme.test{Enter}",
    );
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Reason for contact" }),
      "They asked for it",
    );

    await pressSend(user);

    await waitFor(() => expect(screen.getAllByRole("alert")).toHaveLength(2));
    const saveLine = screen.getByText(/The draft was not saved/);
    expect(
      within(saveLine).getByRole("button", { name: "Retry" }),
    ).toBeTruthy();
  });

  it("is named to the send, which discards it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "POST /emails": () => jsonResponse({ id: "act-9" }, 202),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Reason for contact" }),
      "They asked for it",
    );

    await pressSend(user);

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls(sent, "POST /emails")[0]?.body).toMatchObject({
      mail_draft_id: "md-1",
      body: "Half written before lunch",
    });
    // The server discarded it with the send; the composer asks for nothing.
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(0);
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("is not brought back by a save that answers after the send", async () => {
    let answer: ((response: Response) => void) | undefined;
    const sent = stubRoutes({
      "PUT /mail-drafts": () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    });
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const onClose = vi.fn();
    const { result } = renderHook(
      () =>
        useSavedDraft({
          where: {
            answering: undefined,
            entityType: "contact",
            entityId: "p-1",
            isChannelReply: false,
          },
          open: true,
          fields: {
            to: [],
            cc: [],
            bcc: [],
            subject: "",
            body: "Half written before lunch",
            html: "<p>Half written before lunch</p>",
            files: [],
            context: "",
            sendAt: "",
          },
          replyTo: undefined,
          offeredRecipient: undefined,
          onRestore: () => {},
          onClose,
        }),
      {
        wrapper: ({ children }) => (
          <QueryClientProvider client={client}>
            <LocaleProvider initial="en">{children}</LocaleProvider>
          </QueryClientProvider>
        ),
      },
    );
    await waitFor(() => expect(result.current.enabled).toBe(true));

    act(() => result.current.requestClose());
    await waitFor(() => expect(answer).toBeDefined());
    expect(result.current.saving).toBe(true);
    act(() => result.current.sent());
    act(() => answer?.(jsonResponse(SAVED)));

    await waitFor(() => expect(result.current.saving).toBe(false));
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(1);
    expect(client.getQueryData(["mail-draft", "contact", "p-1"])).toBeNull();
    expect(result.current.held).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
  });
});
