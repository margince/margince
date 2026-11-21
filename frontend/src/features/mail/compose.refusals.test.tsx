/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  fillSendableForm,
  footScheduleConfirm,
  installComposeClock,
  jsonResponse,
  pickWhy,
  pressSend,
  problemResponse,
  renderComposer,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// The 422 body httperr.Validation emits. Every validation problem carries the
// same top-level `code`. The rule that fired is named only in details.errors[],
// beside the field it was asserted about. A stub that hoisted the specific code
// to the top level would prove the screen reads a shape the server never sends.
function validationProblem(field: string, code: string, message: string) {
  return {
    code: "validation_error",
    title: "Unprocessable Entity",
    detail: message,
    details: { errors: [{ field, code, message }] },
  };
}

// The 422 the send answers for a mailbox that cannot send: a message fault
// with its code at the top level and no field named.
const MAILBOX_NOT_SEND_CAPABLE = {
  type: "https://errors.gradion.com/mailbox_not_send_capable",
  title: "Unprocessable Entity",
  status: 422,
  code: "mailbox_not_send_capable",
  detail: "reconnect your mailbox to enable sending",
};

// The 422 for a sender who holds no mailbox at all.
const MAILBOX_NOT_CONNECTED = {
  type: "https://errors.gradion.com/mailbox_not_connected",
  title: "Unprocessable Entity",
  status: 422,
  code: "mailbox_not_connected",
  detail: "connect a mailbox to send",
};

// Each refusal is a product state with a fix, so each reads as its own copy.
// None may fall into the generic line.
describe("ComposeWindow send refusals", () => {
  async function expectReconnect(onClose: () => void) {
    expect(await screen.findByText(/without permission to send/i)).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Reconnect your mailbox" })
        .getAttribute("href"),
    ).toBe("/#/settings/connections");
    expect(screen.queryByText(MAILBOX_NOT_SEND_CAPABLE.detail)).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
  }

  it("tells the rep to reconnect a mailbox that cannot send, and where", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(MAILBOX_NOT_SEND_CAPABLE, 422),
    });
    renderComposer(onClose);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pressSend(user);

    await expectReconnect(onClose);
  });

  it("tells a rep with no mailbox to connect one, not to reconnect", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(MAILBOX_NOT_CONNECTED, 422),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pressSend(user);

    expect(
      await screen.findByText(/No mailbox is connected/i),
    ).toBeInTheDocument();
    expect(
      screen
        .getByRole("link", { name: "Connect a mailbox" })
        .getAttribute("href"),
    ).toBe("/#/settings/connections");
    expect(screen.queryByText(/capture only/i)).toBeNull();
    expect(screen.queryByText(MAILBOX_NOT_CONNECTED.detail)).toBeNull();
  });

  it("says before any send that no mailbox is connected", async () => {
    stubRoutes({ "GET /connectors": () => jsonResponse({ data: [] }) });
    renderComposer();

    expect(
      await screen.findByText(/No mailbox is connected/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Connect a mailbox" }),
    ).toBeInTheDocument();
  });

  it("explains an older server's footer refusal without promising an unusable override", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "body",
            "undisclosable_obligation",
            "this message owes a disclosure (objection_route)",
          ),
          422,
        ),
    });
    renderComposer(onClose);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pressSend(user);
    expect(
      await screen.findByText(/ask an administrator to update Margince/i),
    ).toBeTruthy();
    expect(screen.queryByText(/objection_route/)).toBeNull();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    expect(screen.queryByRole("button", { name: /with a reason/i })).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Subject")).toHaveValue("Hi there");
  });

  it("states the one-send-per-recipient rule on a shared unsubscribe token", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "recipients",
            "shared_unsubscribe_token",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pressSend(user);

    expect(
      await screen.findByText(/goes to one recipient at a time/i),
    ).toBeTruthy();
    expect(screen.queryByText("opaque server wording")).toBeNull();
  });

  it("keeps the generic line for a refusal it has no copy for", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "subject",
            "some_future_refusal",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pressSend(user);

    expect(await screen.findByText(/opaque server wording/i)).toBeTruthy();
  });

  it("tells the rep to reconnect when a scheduled send meets the same refusal", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(MAILBOX_NOT_SEND_CAPABLE, 422),
    });
    renderComposer(onClose);
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await user.click(
      screen.getByRole("button", { name: "Other ways to send" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Schedule send" }),
    );
    await user.click(
      await screen.findByRole("button", { name: /Tomorrow morning/ }),
    );
    await user.click(await footScheduleConfirm());

    await expectReconnect(onClose);
    const scheduled = sent.find(
      (r) => r.key === "POST /activities/act-1/send-email",
    );
    expect(JSON.stringify(scheduled?.body)).toContain("scheduled_at");
  });

  it("uses the server assessment without an extra multi-recipient warning", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes();
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await user.click(screen.getByRole("button", { name: "Cc and Bcc" }));
    await user.type(screen.getByLabelText("Cc"), "second@x.com");
    await user.tab();

    expect(screen.queryByText(/more than one recipient/i)).toBeNull();
    await pickWhy(user, WHY_LABEL.marketing);
    await pressSend(user);

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    expect(screen.queryByText(/more than one recipient/i)).toBeNull();
  });

  it("does not warn about a lone addressee under the same purpose", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    stubRoutes();
    renderComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm(user);
    await pickWhy(user, WHY_LABEL.marketing);

    expect(screen.queryByText(/more than one recipient/i)).toBeNull();
  });
});
