// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { messageBox, writeMessage } from "../../design-system/richtext-testing";
import {
  fillSendableForm,
  installComposeClock,
  jsonResponse,
  pickWhy,
  pressSend,
  problemResponse,
  render,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";
import { ComposeWindow } from "./composewindow";

// Send holds only for what this client knows will stop it: a field the
// message still needs, or an older server's footer refusal. Cmd/Ctrl+Enter is
// the same press and waits for the same holds.

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function openComposer() {
  return render(
    <ComposeWindow
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />,
  );
}

const sendButton = () =>
  screen.getByRole<HTMLButtonElement>("button", { name: "Send" });

const posted = (sent: ReturnType<typeof stubRoutes>) =>
  sent.filter((call) => call.key === "POST /emails");

const pressSendKeys = () => {
  fireEvent.keyDown(messageBox("Body"), { key: "Enter", ctrlKey: true });
  fireEvent.keyDown(messageBox("Body"), { key: "Enter", metaKey: true });
};

describe("a new message without a reason for contact", () => {
  it("is not sent by the button or Cmd/Ctrl+Enter, and goes with the reason once picked", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({ "POST /emails": () => jsonResponse({}, 202) });
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await user.type(screen.getByLabelText("To"), "a@x.com");
    await user.tab();
    await user.type(screen.getByLabelText("Subject"), "Hi there");
    writeMessage("Body", "Body content");

    pressSendKeys();
    await user.click(sendButton());
    expect(
      await screen.findByText("Select the reason for contact."),
    ).toBeInTheDocument();
    expect(posted(sent)).toHaveLength(0);

    await pickWhy(user, WHY_LABEL.marketing);
    fireEvent.keyDown(messageBox("Body"), { key: "Enter", ctrlKey: true });
    await waitFor(() => expect(posted(sent)).toHaveLength(1));
    expect(posted(sent)[0]?.body).toMatchObject({
      communication_context: "marketing",
    });
  });
});

describe("an older server's footer refusal", () => {
  it("holds Send and Cmd/Ctrl+Enter for the refused message and frees them once the message changes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    let presses = 0;
    const sent = stubRoutes({
      "POST /emails": () => {
        presses += 1;
        return presses === 1
          ? problemResponse(
              { code: "public_origin_unusable", title: "Unprocessable" },
              422,
            )
          : jsonResponse({}, 202);
      },
    });
    openComposer();
    await fillSendableForm(user);
    await pressSend(user);
    await screen.findByText(/ask an administrator to update Margince/i);
    expect(sendButton()).toBeDisabled();
    pressSendKeys();

    await user.type(screen.getByLabelText("Subject"), " again");
    await pressSend(user);
    await waitFor(() => expect(presses).toBe(2));
    expect(posted(sent).map((call) => call.body)).toEqual([
      expect.objectContaining({ subject: "Hi there" }),
      expect.objectContaining({ subject: "Hi there again" }),
    ]);
  });
});
