// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ComposeModal } from "./compose";
import {
  fillSendableForm,
  jsonResponse,
  problemResponse,
  render,
  stubRoutes,
} from "./compose.testkit";

// A message sent past its refusal as a recorded exception is a sent message.
// The composer closes the way a normal send closes it, so nothing on screen
// invites a second send of mail that has gone.

const REVIEW = {
  id: "r-1",
  state: "open",
  kind: "single",
  reason_code: "no_compatible_evidence",
  refusals: [],
  warning: {
    version: "override-v2",
    text: "The send check still blocks this message.",
  },
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("sending with a recorded exception", () => {
  it("closes the composer once the exception has sent the message", async () => {
    const sent = stubRoutes({
      "POST /emails": () =>
        problemResponse(
          {
            code: "consent_not_granted",
            title: "Conflict",
            details: {
              review_id: "r-1",
              available_actions: ["direct_send"],
              refusals: [],
            },
          },
          409,
        ),
      "GET /communication-reviews/r-1": () => jsonResponse(REVIEW),
      "POST /communication-reviews/r-1/direct-send": () =>
        jsonResponse({ id: "act-9" }),
    });
    const onClose = vi.fn();
    const onSent = vi.fn();
    render(
      <ComposeModal
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={onClose}
        onSent={onSent}
      />,
    );
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await userEvent.click(
      await screen.findByRole("button", {
        name: "Send with a recorded exception",
      }),
    );
    await userEvent.click(await screen.findByRole("checkbox"));
    await userEvent.type(
      screen.getByLabelText("Explanation"),
      "They asked for it on the phone.",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Record the exception and send" }),
    );

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(onSent).toHaveBeenCalledTimes(1);
    expect(sent.filter((s) => s.key === "POST /emails")).toHaveLength(1);
    expect(
      sent.filter(
        (s) => s.key === "POST /communication-reviews/r-1/direct-send",
      ),
    ).toHaveLength(1);
  });
});
