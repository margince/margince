// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { entityTimelineKeys } from "../../lib/activitykeys";
import { ComposeModal } from "./compose";
import {
  fillSendableForm,
  jsonResponse,
  problemResponse,
  render,
  stubRoutes,
} from "./compose.testkit";
import { draftKey } from "./composesaveddraftcalls";

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
    const saved = {
      id: "md-1",
      anchor_type: "contact",
      anchor_id: "c-1",
      to: [],
      cc: [],
      bcc: [],
      subject: "Before lunch",
      body: "Earlier words.",
      html_body: "<p>Earlier words.</p>",
      version: 1,
      agent_drafted: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(saved),
      "PUT /mail-drafts": () => jsonResponse({ ...saved, version: 2 }),
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
    function Composer() {
      const [open, setOpen] = useState(true);
      return (
        <ComposeModal
          entityType="contact"
          entityId="c-1"
          contactId="c-1"
          open={open}
          onClose={() => {
            setOpen(false);
            onClose();
          }}
          onSent={onSent}
        />
      );
    }
    const { queryClient } = render(
      <ToastProvider>
        <Composer />
        <ToastRegion />
      </ToastProvider>,
    );
    const timelineKeys = entityTimelineKeys("contact", "c-1");
    for (const key of timelineKeys) queryClient.setQueryData(key, []);
    await screen.findByText("Saved draft restored");
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
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByText("Queued as a recorded exception.")).toBeTruthy();
    expect(
      queryClient.getQueryData(draftKey({ type: "contact", id: "c-1" })),
    ).toBeNull();
    expect(timelineKeys.length).toBeGreaterThan(0);
    for (const key of timelineKeys.slice(0, 1)) {
      expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
    }
    expect(onSent).toHaveBeenCalledTimes(1);
    expect(sent.find((s) => s.key === "POST /emails")?.body).toMatchObject({
      mail_draft_id: saved.id,
    });
    expect(sent.filter((s) => s.key === "POST /emails")).toHaveLength(1);
    expect(
      sent.filter(
        (s) => s.key === "POST /communication-reviews/r-1/direct-send",
      ),
    ).toHaveLength(1);
  });
});
