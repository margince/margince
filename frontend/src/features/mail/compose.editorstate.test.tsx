/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { messageText } from "../../design-system/richtext-testing";
import { ComposeModal } from "./compose";
import { jsonResponse, render, stubRoutes } from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("restores the purpose and AI attribution without generating again", async () => {
  const user = userEvent.setup();
  const body = "Hallo Dana, passt die neue Laufzeit für euch?";
  const sent = stubRoutes({
    "GET /mail-drafts": () =>
      jsonResponse({
        id: "md-ai",
        anchor_type: "contact",
        anchor_id: "p-1",
        to: ["dana@example.test"],
        cc: [],
        bcc: [],
        subject: "Neue Laufzeit",
        body,
        version: 1,
        agent_drafted: false,
        created_at: "2026-09-20T09:00:00Z",
        updated_at: "2026-09-20T09:00:00Z",
        editor_state: {
          intent: "Ask about the new term",
          ai_generated: true,
          ai_disclosure: "Mit KI-Unterstützung verfasst.",
          served_body: body,
        },
      }),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
      draftOnOpen
      open
      onClose={vi.fn()}
    />,
  );
  await screen.findByText("Saved draft restored");
  expect(messageText("Body")).toBe(body);
  expect(
    await screen.findByRole("heading", { name: "AI-assisted draft" }),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Change the ask" }));
  expect(
    await screen.findByDisplayValue("Ask about the new term"),
  ).toBeTruthy();
  expect(sent.some((call) => call.key.endsWith("/draft-email"))).toBe(false);
});

it("passes the written subject with an explicit new-email generation", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes({
    "POST /contacts/p-1/draft-email": () =>
      jsonResponse({
        subject: "Renewal",
        body: "Hello Dana,\n\nDoes the renewal work?",
        ai_generated: true,
      }),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="p-1"
      intent="Ask about renewal"
      open
      onClose={vi.fn()}
    />,
  );
  await user.type(
    await screen.findByLabelText("Subject"),
    "Verlängerung der Laufzeit",
  );
  expect(sent.some((call) => call.key.endsWith("/draft-email"))).toBe(false);
  await user.click(screen.getByRole("button", { name: "Draft email" }));
  await waitFor(() =>
    expect(
      sent.find((call) => call.key === "POST /contacts/p-1/draft-email")?.body,
    ).toMatchObject({
      intent: "Ask about renewal",
      subject: "Verlängerung der Laufzeit",
    }),
  );
  expect(screen.getByDisplayValue("Verlängerung der Laufzeit")).toBeTruthy();
});

it("does not fill a draft that answered an earlier subject", async () => {
  const user = userEvent.setup();
  let finish: (response: Response) => void = () => {
    throw new Error("request not started");
  };
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const sent = stubRoutes({ "POST /contacts/p-1/draft-email": () => pending });
  render(
    <ComposeModal
      entityType="contact"
      entityId="p-1"
      intent="Ask about renewal"
      open
      onClose={vi.fn()}
    />,
  );
  const subject = await screen.findByLabelText("Subject");
  await user.type(subject, "Renewal");
  await user.click(screen.getByRole("button", { name: "Draft email" }));
  await waitFor(() =>
    expect(sent.some((call) => call.key.endsWith("/draft-email"))).toBe(true),
  );
  await user.clear(subject);
  await user.type(subject, "A different request");
  finish(
    jsonResponse({
      subject: "Renewal",
      body: "The earlier renewal draft.",
      ai_generated: true,
    }),
  );
  await screen.findByText("Your edits were kept. Draft again when ready.");
  expect(messageText("Body")).toBe("");
  expect(screen.getByDisplayValue("A different request")).toBeTruthy();
});

it("keeps a response when only the project tag changes", async () => {
  const user = userEvent.setup();
  let finish: (response: Response) => void = () => {
    throw new Error("request not started");
  };
  const pending = new Promise<Response>((resolve) => {
    finish = resolve;
  });
  const sent = stubRoutes({ "POST /contacts/p-1/draft-email": () => pending });
  render(
    <ComposeModal
      entityType="contact"
      entityId="p-1"
      intent="Ask about renewal"
      open
      onClose={vi.fn()}
    />,
  );
  const subject = await screen.findByLabelText("Subject");
  await user.type(subject, "Renewal");
  await user.click(screen.getByRole("button", { name: "Draft email" }));
  await waitFor(() =>
    expect(sent.some((call) => call.key.endsWith("/draft-email"))).toBe(true),
  );
  await user.clear(subject);
  await user.type(subject, "[[ERP-1] Renewal");
  finish(
    jsonResponse({
      subject: "Renewal",
      body: "Hello Dana, does the renewal work?",
      ai_generated: true,
    }),
  );
  await waitFor(() =>
    expect(messageText("Body")).toBe("Hello Dana, does the renewal work?"),
  );
  expect(
    screen.queryByText("Your edits were kept. Draft again when ready."),
  ).toBeNull();
});
