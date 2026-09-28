/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { messageText, writeMessage } from "../design-system/richtext-testing";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

type Activity = components["schemas"]["Activity"];
function email(
  id: string,
  subject: string,
  direction: "inbound" | "outbound" = "inbound",
): Activity {
  return {
    id,
    kind: "email",
    subject,
    body: `Full text of ${subject}.\n\nA second paragraph beyond the preview.`,
    direction,
    thread_key: "pricing",
    occurred_at: "2026-09-01T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    content_state: "available",
    email_summary: {
      activity_id: id,
      subject,
      preview: "Short preview",
      counterparty: "Ada",
      occurred_at: "2026-09-01T09:00:00Z",
      direction,
      display_status: "team",
      attachment_count: 0,
      move: "needs_reply",
      version: 1,
    },
  };
}
const first = email("a1", "Pricing");
const second = email("a2", "Re: Delivery", "outbound");
function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });
}

function setup(
  draftReply?: () => Promise<Response>,
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
  anchor = first,
  activityId?: string,
) {
  const requests: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request
          ? input
          : new Request(new URL(String(input), "https://test.local"), init);
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      const body: unknown =
        request.method === "GET" ? undefined : await request.clone().json();
      if (request.method !== "GET") requests.push({ path, body });
      if (isPreviewDoor(path))
        return json(allowedPreview(previewedAddresses(body)));
      if (path.endsWith("/draft-email"))
        return draftReply
          ? draftReply()
          : json({
              subject: "Re: Delivery",
              body: "The generated reply.",
              to: ["ada@example.test"],
            });
      if (path.endsWith("/reply-recipient"))
        return json({ address: "ada@example.test", mailbox_user_ids: [] });
      if (path === "/activities/a1") return json(anchor);
      if (path === "/activities/a2") return json(second);
      if (path === "/activities")
        return json({ data: [anchor, second], page: { has_more: false } });
      if (path === "/voice-profiles") return json({ data: [] });
      return json({});
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ComposeModal
          activityId={activityId}
          entityType="contact"
          entityId="c1"
          contactId="c1"
          open
          onClose={vi.fn()}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return requests;
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("requires a purpose for a new AI email, then sends that purpose to the contact writer", async () => {
  const user = userEvent.setup();
  const requests = setup();
  const draft = screen.getByRole("button", { name: "Draft with AI" });
  expect(draft.hasAttribute("disabled")).toBe(true);
  await user.type(
    screen.getByPlaceholderText("Purpose of the email"),
    "Offer a discovery call",
  );
  await user.click(draft);
  await waitFor(() =>
    expect(requests).toContainEqual({
      path: "/contacts/c1/draft-email",
      body: { intent: "Offer a discovery call" },
    }),
  );
});

it("selects the exact message, normalizes the reply subject, and restores each draft when switching", async () => {
  const user = userEvent.setup();
  const requests = setup();
  writeMessage("Body", "My new email");
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await waitFor(() => {
    expect(screen.getByDisplayValue("Re: Pricing")).toBeTruthy();
    expect(messageText("Body")).toBe("");
  });
  writeMessage("Body", "Answer to pricing");
  await user.click(screen.getByRole("button", { name: /Re: Delivery/ }));
  await waitFor(() => {
    expect(screen.getByDisplayValue("Re: Delivery")).toBeTruthy();
    expect(messageText("Body")).toBe("");
  });
  expect(screen.getByText(/Following up on your email/)).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Draft reply with AI" }));
  await waitFor(() => expect(messageText("Body")).toBe("The generated reply."));
  expect(
    requests.some((request) => request.path === "/activities/a2/draft-email"),
  ).toBe(true);
  await user.click(screen.getByRole("button", { name: /Pricing/ }));
  await waitFor(() => expect(messageText("Body")).toBe("Answer to pricing"));
  expect(
    screen
      .getByRole("button", { name: /Pricing/ })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  await user.click(screen.getByRole("button", { name: "New email" }));
  await waitFor(() => expect(messageText("Body")).toBe("My new email"));
});

it("ignores a delayed draft after selecting another message", async () => {
  const user = userEvent.setup();
  let finish: ((response: Response) => void) | undefined;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const requests = setup(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
    client,
  );
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await screen.findByDisplayValue("Re: Pricing");
  await user.click(screen.getByRole("button", { name: "Draft reply with AI" }));
  await waitFor(() =>
    expect(
      requests.some((request) => request.path === "/activities/a1/draft-email"),
    ).toBe(true),
  );
  await user.click(screen.getByRole("button", { name: /Re: Delivery/ }));
  await screen.findByDisplayValue("Re: Delivery");
  writeMessage("Body", "Keep my follow-up");
  finish?.(
    json({
      subject: "Re: Pricing",
      body: "Late answer",
      to: ["wrong@example.test"],
    }),
  );
  await waitFor(() => expect(client.isMutating()).toBe(0));
  expect(messageText("Body")).toBe("Keep my follow-up");
  expect(screen.queryByText("wrong@example.test")).toBeNull();
});

it("previews full paragraphs without changing the reply target", async () => {
  const user = userEvent.setup();
  setup();
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await screen.findByDisplayValue("Re: Pricing");
  fireEvent.click(
    within(screen.getByRole("listitem", { name: /Re: Delivery/ })).getByRole(
      "button",
      { name: "Preview" },
    ),
  );
  expect(await screen.findByText(/Full text of Re: Delivery\./)).toBeTruthy();
  expect(
    screen.getByText(/A second paragraph beyond the preview\./),
  ).toBeTruthy();
  expect(screen.getByDisplayValue("Re: Pricing")).toBeTruthy();
});

it("keeps edits made while the selected email's AI draft is in flight", async () => {
  const user = userEvent.setup();
  let finish: ((response: Response) => void) | undefined;
  setup(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await screen.findByDisplayValue("Re: Pricing");
  await user.click(screen.getByRole("button", { name: "Draft reply with AI" }));
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  writeMessage("Body", "My answer written while waiting");
  finish?.(json({ subject: "Re: Pricing", body: "Late generated text" }));
  await screen.findByText("Your edits were kept. Draft again when ready.");
  expect(messageText("Body")).toBe("My answer written while waiting");
});

it("preserves recipient choices and instructions separately for each reply", async () => {
  const user = userEvent.setup();
  setup();
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await screen.findByDisplayValue("Re: Pricing");
  await user.type(
    screen.getByRole("combobox", { name: "Cc" }),
    "finance@example.test{Enter}",
  );
  await user.click(screen.getByRole("button", { name: "Bcc" }));
  await user.type(
    screen.getByRole("combobox", { name: "Bcc" }),
    "archive@example.test{Enter}",
  );
  await user.type(
    screen.getByPlaceholderText("Purpose of the reply"),
    "Confirm the price",
  );
  await user.click(screen.getByRole("button", { name: /Re: Delivery/ }));
  await screen.findByDisplayValue("Re: Delivery");
  expect(screen.queryByText("finance@example.test")).toBeNull();
  expect(screen.queryByText("archive@example.test")).toBeNull();
  await user.click(screen.getByRole("button", { name: /Pricing/ }));
  expect(screen.getByText("finance@example.test")).toBeTruthy();
  expect(screen.getByText("archive@example.test")).toBeTruthy();
  expect(screen.getByDisplayValue("Confirm the price")).toBeTruthy();
});

it("leaves a subjectless reply editable without inserting a display placeholder", async () => {
  setup(undefined, undefined, email("a1", ""), "a1");
  await screen.findByText(/Replying to/);
  expect(
    screen.getByRole("textbox", { name: "Subject" }).getAttribute("value"),
  ).toBe("");
});

it("retries a failed selected-message read before enabling reply drafting", async () => {
  const user = userEvent.setup();
  setup();
  const fetcher = vi.mocked(fetch);
  const original = fetcher.getMockImplementation();
  if (!original) throw new Error("Missing test transport");
  let failed = false;
  fetcher.mockImplementation((input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.endsWith("/activities/a1") && !failed) {
      failed = true;
      return Promise.resolve(
        new Response(JSON.stringify({ title: "Unavailable" }), {
          status: 503,
          headers: { "Content-Type": "application/problem+json" },
        }),
      );
    }
    return original(input, init);
  });
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await screen.findByText("This section did not load.");
  expect(
    screen
      .getByRole("button", { name: "Draft reply with AI" })
      .hasAttribute("disabled"),
  ).toBe(true);
  await user.click(screen.getByRole("button", { name: "Retry" }));
  await screen.findByDisplayValue("Re: Pricing");
  expect(
    screen
      .getByRole("button", { name: "Draft reply with AI" })
      .hasAttribute("disabled"),
  ).toBe(false);
});

it("keeps withheld thread entries unreadable and unavailable as reply targets", async () => {
  setup(
    undefined,
    undefined,
    { ...first, content_state: "withheld", body: undefined },
    "a2",
  );
  await screen.findByDisplayValue("Re: Delivery");
  const held = within(await screen.findByRole("listitem", { name: /Pricing/ }));
  expect(held.queryByRole("button")).toBeNull();
  expect(held.queryByText(/Full text/)).toBeNull();
});

it("files a delayed upload on the draft that started it, even after switching", async () => {
  const user = userEvent.setup();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  setup(undefined, client);
  const fetcher = vi.mocked(fetch);
  const original = fetcher.getMockImplementation();
  if (!original) throw new Error("Missing test transport");
  let finish: ((response: Response) => void) | undefined;
  fetcher.mockImplementation((input, init) => {
    if (input === "/v1/attachments" && init?.method === "POST") {
      return new Promise<Response>((resolve) => {
        finish = resolve;
      });
    }
    return original(input, init);
  });
  await user.click(await screen.findByRole("button", { name: /Pricing/ }));
  await screen.findByDisplayValue("Re: Pricing");
  await user.click(screen.getByRole("button", { name: /Attach/ }));
  await user.upload(
    screen.getByLabelText("Upload file"),
    new File(["Offer"], "pricing.txt", { type: "text/plain" }),
  );
  await waitFor(() => expect(finish).toBeTypeOf("function"));
  await user.click(screen.getByRole("button", { name: /Re: Delivery/ }));
  await screen.findByDisplayValue("Re: Delivery");
  finish?.(json({ id: "file-a", filename: "pricing.txt", byte_size: 5 }));
  await waitFor(() => expect(client.isMutating()).toBe(0));
  expect(screen.queryByText(/pricing.txt/)).toBeNull();
  await user.click(screen.getByRole("button", { name: /Pricing/ }));
  expect(await screen.findByText(/pricing.txt/)).toBeTruthy();
});
