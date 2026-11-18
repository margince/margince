/** @vitest-environment happy-dom */
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { UTC_ZONE } from "../../format/timezone";
import { pressSend } from "./compose.testkit";
import {
  calls,
  composer,
  jsonResponse,
  type MailDraft,
  problem,
  type Route,
  render,
  SAVED,
  type Sent,
  stubRoutes,
} from "./compose-saveddraft.testkit";
import { ComposeWindow } from "./composewindow";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("a saved draft put back whole", () => {
  const WHOLE: MailDraft = {
    ...SAVED,
    attachments: [
      {
        id: "f-1",
        filename: "seat-pricing.pdf",
        content_type: "application/pdf",
        byte_size: 48_213,
      },
    ],
    communication_context: "requested_followup",
    scheduled_at: "2026-11-20T08:00:00Z",
    scheduled_tz: UTC_ZONE,
  };

  it("brings back its files, its reason for contact and its send time", async () => {
    const sent = stubRoutes({ "GET /mail-drafts": () => jsonResponse(WHOLE) });
    render(composer(vi.fn()));
    await screen.findByText("Saved draft restored");

    expect(screen.getByText("seat-pricing.pdf")).toBeTruthy();
    expect(
      screen.getByRole("combobox", { name: "Reason for contact" }).textContent,
    ).toContain("Follow-up they requested");
    expect(screen.getByRole("button", { name: "Schedule send" })).toBeTruthy();
    expect(screen.getByText(/^Sends /)).toBeTruthy();
    // What came back is what the draft holds, so nothing is written again.
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("counts the files that could not come back, and names none", async () => {
    stubRoutes({
      "GET /mail-drafts": () =>
        jsonResponse({ ...WHOLE, attachments_unavailable: 2 }),
    });
    render(composer(vi.fn()));

    expect(
      await screen.findByText("2 files could not be restored."),
    ).toBeTruthy();
    expect(screen.getByText("seat-pricing.pdf")).toBeTruthy();
  });

  it("marks an agent's draft as the agent's, files and all", async () => {
    stubRoutes({
      "GET /mail-drafts": () =>
        jsonResponse({
          ...WHOLE,
          agent_drafted: true,
          attachments_unavailable: 1,
        }),
    });
    render(composer(vi.fn()));

    expect(
      await screen.findByText("Agent draft restored. Read it before sending."),
    ).toBeTruthy();
    expect(screen.getByText("1 file could not be restored.")).toBeTruthy();
    expect(screen.getByText("seat-pricing.pdf")).toBeTruthy();
  });

  it("offers Retry under a refused save, and the retry saves and closes", async () => {
    const onClose = vi.fn();
    let puts = 0;
    const sent = stubRoutes({
      "PUT /mail-drafts": () => {
        puts += 1;
        return puts === 1 ? problem("internal", 503) : jsonResponse(SAVED);
      },
    });
    const user = userEvent.setup();
    render(composer(onClose));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");
    await user.click(screen.getByRole("button", { name: "Close" }));
    const line = await screen.findByRole("alert");

    await user.click(within(line).getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(2);
    expect(messageText("Body")).toBe("Half written before lunch");
  });
});

describe("a composer opened with a caller's own words", () => {
  const proposal = (onClose: () => void) => (
    <ComposeWindow
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
      initialMessage={{ subject: "Times to meet", body: "Pick a time" }}
      open
      onClose={onClose}
    />
  );

  it("closes without saving over the record's draft", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({ "GET /mail-drafts": () => jsonResponse(SAVED) });
    render(proposal(onClose));
    await waitFor(() => expect(messageText("Body")).toContain("Pick a time"));
    writeMessage("Body", "Pick a time, Ada");

    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(onClose).toHaveBeenCalled();
    expect(calls(sent, "GET /mail-drafts")).toHaveLength(0);
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("sends without naming the record's draft", async () => {
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "POST /emails": () => jsonResponse({ id: "act-9" }, 202),
    });
    render(proposal(vi.fn()));
    await userEvent.type(
      await screen.findByRole("combobox", { name: "To" }),
      "buyer@acme.test{enter}",
    );
    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "Reason for contact" }),
      "Follow-up they requested",
    );

    await pressSend();

    await waitFor(() => expect(calls(sent, "POST /emails")).toHaveLength(1));
    expect(calls(sent, "POST /emails")[0]?.body).not.toHaveProperty(
      "mail_draft_id",
    );
  });
});
