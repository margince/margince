/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { viewerZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { jsonResponse, stubRoutes } from "./compose.testkit";
import { ScheduledSendsScreen } from "./scheduledsends";

const readerZone = viewerZone();

const NEW_MESSAGE = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  status: "scheduled",
  scheduled_at: "2026-09-01T07:00:00Z",
  scheduled_tz: readerZone,
  subject: "The renewal quote",
  to: ["ceo@acme.test"],
  cc: ["cfo@acme.test"],
  body: "Here is the quote.",
  links: [{ entity_type: "company", entity_id: "company-1" }],
  communication_context: "requested_followup",
  version: 3,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

const REPLY = {
  ...NEW_MESSAGE,
  id: "019f7e65-fbf7-7114-b114-40af4af63ae9",
  subject: "Re: the quote",
  anchor_activity_id: "019f7e65-fbf7-7114-b114-40af4af63aff",
  links: undefined,
};

const THREAD_EMAIL = {
  id: REPLY.anchor_activity_id,
  kind: "email",
  direction: "inbound",
  subject: "The quote",
  content_state: "available",
  occurred_at: "2026-08-19T09:00:00Z",
  links: [{ entity_type: "company", entity_id: "company-1" }],
};

const MARKETING = {
  ...NEW_MESSAGE,
  id: "019f7e65-fbf7-7114-b114-40af4af63aea",
  subject: "The newsletter",
  marketing_purpose: "marketing_email",
};

const FORMATTED = {
  ...NEW_MESSAGE,
  html_body:
    '<p>Here is the <strong>quote</strong>, see <a href="https://acme.test/terms">the terms</a>.</p>',
  attachments: [
    {
      id: "019f7e65-fbf7-7114-b114-40af4af63b01",
      filename: "quote.pdf",
      content_type: "application/pdf",
      byte_size: 2048,
    },
    {
      id: "019f7e65-fbf7-7114-b114-40af4af63b02",
      filename: "terms.pdf",
      content_type: "application/pdf",
      byte_size: 1024,
    },
  ],
  draft_ref: "draft-ref-1",
};

type Row = Readonly<Record<string, unknown> & { id: string }>;

// The list, and each message's own read, which an edit opens from.
function queueRoutes(rows: readonly Row[], details: readonly Row[] = rows) {
  return {
    "GET /scheduled-sends": () => jsonResponse(rows),
    ...Object.fromEntries(
      details.map((row) => [
        `GET /scheduled-sends/${row.id}`,
        () => jsonResponse(row),
      ]),
    ),
  };
}

let client: QueryClient;

function mount(
  rows: readonly Row[],
  details: readonly Row[] = rows,
  extra: Parameters<typeof stubRoutes>[0] = {},
) {
  const sent = stubRoutes({
    ...queueRoutes(rows, details),
    ...extra,
    "POST /emails": () => jsonResponse({ id: "act-new" }, 201),
    [`POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`]: () =>
      new Response(null, { status: 204 }),
  });
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return sent;
}

const THREAD_ROUTE = {
  [`GET /activities/${REPLY.anchor_activity_id}`]: () =>
    jsonResponse(THREAD_EMAIL),
};

// A reply's thread has answered, and every effect of that answer has run.
async function settled(row: Row) {
  if (row.anchor_activity_id !== undefined) {
    await waitFor(() =>
      expect(
        client.getQueryState(["activity", row.anchor_activity_id, "filing"])
          ?.status,
      ).toBe("success"),
    );
  }
  await act(async () => {});
}

const ANCHOR_GONE = /no longer available to reply to/;

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"], now: new Date("2026-08-31T10:00:00Z") });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

it("offers Edit on a new message and a reply, not on a marketing send", async () => {
  mount([NEW_MESSAGE, REPLY, MARKETING]);
  await screen.findByText("The renewal quote");
  expect(screen.getAllByRole("button", { name: /^Edit: / })).toHaveLength(2);
  expect(
    screen.getByRole("button", { name: "Edit: The renewal quote" }),
  ).toBeVisible();
});

it("opens the composer on the message as it was scheduled", async () => {
  const user = userEvent.setup();
  mount([NEW_MESSAGE]);
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  expect(await screen.findByLabelText("Subject")).toHaveValue(
    "The renewal quote",
  );
  const composer = within(await screen.findByRole("dialog"));
  expect(composer.getByText("ceo@acme.test")).toBeVisible();
  expect(composer.getByText("cfo@acme.test")).toBeVisible();
});

it("schedules the edited message first and withdraws the old one after", async () => {
  const user = userEvent.setup();
  const sent = mount([NEW_MESSAGE]);
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  const subject = await screen.findByLabelText("Subject");
  await user.clear(subject);
  await user.type(subject, "The renewal quote, revised");
  writeMessage("Body", "Here is the revised quote.");
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.map((call) => call.key)).toContain(
      `POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`,
    ),
  );
  const keys = sent.map((call) => call.key);
  const create = sent.find((call) => call.key === "POST /emails");
  expect(create?.body).toMatchObject({
    subject: "The renewal quote, revised",
    to: ["ceo@acme.test"],
    cc: ["cfo@acme.test"],
    scheduled_tz: readerZone,
  });
  expect(keys.indexOf("POST /emails")).toBeLessThan(
    keys.indexOf(`POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`),
  );
});

it("leaves the original waiting when the new send is refused", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes({
    ...queueRoutes([NEW_MESSAGE]),
    "POST /emails": () =>
      new Response(
        JSON.stringify({ type: "about:blank", title: "No", status: 422 }),
        {
          status: 422,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.some((call) => call.key === "POST /emails")).toBe(true),
  );
  expect(sent.some((call) => call.key.endsWith("/cancel"))).toBe(false);
});

it("writes a reply again on its thread and withdraws the old one after", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes({
    ...queueRoutes([REPLY]),
    [`GET /activities/${REPLY.anchor_activity_id}`]: () =>
      jsonResponse(THREAD_EMAIL),
    [`POST /activities/${REPLY.anchor_activity_id}/send-email`]: () =>
      jsonResponse({ id: "act-new" }, 201),
    [`POST /scheduled-sends/${REPLY.id}/cancel`]: () =>
      new Response(null, { status: 204 }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Edit: Re: the quote" }),
  );
  expect(await screen.findByLabelText("Subject")).toHaveValue("Re: the quote");
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.map((call) => call.key)).toContain(
      `POST /scheduled-sends/${REPLY.id}/cancel`,
    ),
  );
  const reply = sent.find((call) => call.key.endsWith("/send-email"));
  expect(reply?.body).toMatchObject({ to: ["ceo@acme.test"] });
  expect(sent.some((call) => call.key === "POST /emails")).toBe(false);
});

it("says so and opens nothing when the reply's thread has no record to file under", async () => {
  const user = userEvent.setup();
  stubRoutes({
    ...queueRoutes([REPLY]),
    [`GET /activities/${REPLY.anchor_activity_id}`]: () =>
      jsonResponse({ ...THREAD_EMAIL, links: [] }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Edit: Re: the quote" }),
  );
  expect(
    await screen.findByText(/no longer available to reply to/),
  ).toBeVisible();
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("opens on the message's own read and keeps its markup, files and draft when saved unchanged", async () => {
  const user = userEvent.setup();
  const sent = mount([NEW_MESSAGE], [FORMATTED]);
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  const composer = within(await screen.findByRole("dialog"));
  expect(composer.getByText("quote.pdf")).toBeVisible();
  expect(composer.getByText("terms.pdf")).toBeVisible();
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.map((call) => call.key)).toContain(
      `POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`,
    ),
  );
  const create = sent.find((call) => call.key === "POST /emails");
  const body = create?.body as Record<string, unknown>;
  expect(body.html_body).toContain("<strong>quote</strong>");
  expect(body.html_body).toContain('href="https://acme.test/terms"');
  expect(body).toMatchObject({
    attachment_ids: FORMATTED.attachments.map((file) => file.id),
    draft_ref: "draft-ref-1",
  });
});

it("sends without a seeded file the reader removed", async () => {
  const user = userEvent.setup();
  const sent = mount([NEW_MESSAGE], [FORMATTED]);
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Remove quote.pdf" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Schedule send" }),
  );
  await waitFor(() =>
    expect(sent.some((call) => call.key === "POST /emails")).toBe(true),
  );
  const create = sent.find((call) => call.key === "POST /emails");
  expect(create?.body).toMatchObject({
    attachment_ids: [FORMATTED.attachments[1]?.id],
  });
});

it("refuses Edit for an operator reason, says why and still withdraws the message", async () => {
  const user = userEvent.setup();
  const reason = { operator_reason: "Asked for it at the fair" };
  const sent = mount([
    { ...NEW_MESSAGE, ...reason },
    { ...MARKETING, ...reason },
  ]);
  const edit = await screen.findByRole("button", {
    name: "Edit: The renewal quote",
  });
  expect(edit).toBeDisabled();
  expect(edit).toHaveAccessibleDescription(
    "Not editable: an edit would drop the reason you gave when scheduling it. Withdraw the message and schedule a new one.",
  );
  expect(screen.getAllByRole("button", { name: /^Edit: / })).toHaveLength(1);
  await user.click(
    screen.getByRole("button", { name: "Withdraw: The renewal quote" }),
  );
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: "Withdraw message",
    }),
  );
  await waitFor(() =>
    expect(sent.map((call) => call.key)).toContain(
      `POST /scheduled-sends/${NEW_MESSAGE.id}/cancel`,
    ),
  );
});

it.each([
  ["a new message", NEW_MESSAGE],
  ["a reply", REPLY],
])(
  "says why in place of the composer when the read of %s names a file the send would refuse",
  async (_, row) => {
    const user = userEvent.setup();
    mount([row], [{ ...row, attachments_unavailable: 1 }], THREAD_ROUTE);
    await user.click(
      await screen.findByRole("button", { name: `Edit: ${row.subject}` }),
    );
    const refusal = within(await screen.findByRole("dialog"));
    expect(
      await refusal.findByText(
        "1 attached file can no longer be sent. Withdraw the message and schedule a new one.",
      ),
    ).toBeVisible();
    await settled(row);
    expect(screen.getByRole("dialog")).toBeVisible();
    expect(screen.queryByText(ANCHOR_GONE)).toBeNull();
    expect(screen.queryByLabelText("Subject")).toBeNull();
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Close" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(
      screen.getByRole("button", { name: `Withdraw: ${row.subject}` }),
    ).toBeEnabled();
  },
);

it("offers a retry when the message's own read fails, and opens once it lands", async () => {
  const user = userEvent.setup();
  let reads = 0;
  mount([NEW_MESSAGE], [], {
    [`GET /scheduled-sends/${NEW_MESSAGE.id}`]: () => {
      reads += 1;
      return reads === 1
        ? new Response(
            JSON.stringify({ type: "about:blank", title: "Down", status: 503 }),
            {
              status: 503,
              headers: { "Content-Type": "application/problem+json" },
            },
          )
        : jsonResponse(NEW_MESSAGE);
    },
  });
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  const failed = within(await screen.findByRole("dialog"));
  expect(
    await failed.findByText("Could not load this view. Reload the page."),
  ).toBeVisible();
  await user.click(failed.getByRole("button", { name: "Retry" }));
  expect(await screen.findByLabelText("Subject")).toHaveValue(
    "The renewal quote",
  );
});

it("holds the composer in a loading state until the message's own read lands", async () => {
  const user = userEvent.setup();
  let land: (response: Response) => void = () => {};
  mount([NEW_MESSAGE], [], {
    [`GET /scheduled-sends/${NEW_MESSAGE.id}`]: () =>
      new Promise<Response>((resolve) => {
        land = resolve;
      }),
  });
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  const loading = within(await screen.findByRole("dialog"));
  expect(loading.getByText("Loading message…")).toBeInTheDocument();
  land(jsonResponse(NEW_MESSAGE));
  expect(await screen.findByLabelText("Subject")).toHaveValue(
    "The renewal quote",
  );
});

it("opens a reply whose thread answers before the message's own read does", async () => {
  const user = userEvent.setup();
  let land: (response: Response) => void = () => {};
  mount([REPLY], [], {
    ...THREAD_ROUTE,
    [`GET /scheduled-sends/${REPLY.id}`]: () =>
      new Promise<Response>((resolve) => {
        land = resolve;
      }),
  });
  await user.click(
    await screen.findByRole("button", { name: "Edit: Re: the quote" }),
  );
  await settled(REPLY);
  land(jsonResponse(REPLY));
  expect(await screen.findByLabelText("Subject")).toHaveValue("Re: the quote");
  expect(screen.queryByText(ANCHOR_GONE)).toBeNull();
});

it.each([
  ["a new message that has gone", NEW_MESSAGE, { status: "sent" }],
  ["a reply that has gone", REPLY, { status: "sent" }],
  [
    "a message that now carries an operator reason",
    NEW_MESSAGE,
    { operator_reason: "Asked at the fair" },
  ],
])(
  "says so in place of the composer for %s since the list was read, and rereads the list",
  async (_, row, change) => {
    const user = userEvent.setup();
    const sent = mount([row], [{ ...row, ...change }], THREAD_ROUTE);
    await user.click(
      await screen.findByRole("button", { name: `Edit: ${row.subject}` }),
    );
    const refusal = within(await screen.findByRole("dialog"));
    expect(await refusal.findByText("Message changed")).toBeVisible();
    await waitFor(() =>
      expect(
        sent.filter((call) => call.key === "GET /scheduled-sends"),
      ).toHaveLength(2),
    );
    await settled(row);
    expect(screen.getByRole("dialog")).toBeVisible();
    expect(screen.queryByText(ANCHOR_GONE)).toBeNull();
    expect(screen.queryByLabelText("Subject")).toBeNull();
  },
);

it("reads a message whose own read answers 404 as changed, not as a failed load", async () => {
  const user = userEvent.setup();
  const sent = mount([NEW_MESSAGE], [], {
    [`GET /scheduled-sends/${NEW_MESSAGE.id}`]: () =>
      new Response(
        JSON.stringify({ type: "about:blank", title: "Gone", status: 404 }),
        {
          status: 404,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
  });
  await user.click(
    await screen.findByRole("button", { name: "Edit: The renewal quote" }),
  );
  const refusal = within(await screen.findByRole("dialog"));
  expect(await refusal.findByText("Message changed")).toBeVisible();
  expect(refusal.queryByRole("button", { name: "Retry" })).toBeNull();
  await waitFor(() =>
    expect(
      sent.filter((call) => call.key === "GET /scheduled-sends"),
    ).toHaveLength(2),
  );
});

it.each([
  [
    "fails",
    () =>
      new Response(
        JSON.stringify({ type: "about:blank", title: "Down", status: 500 }),
        {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
  ],
  [
    "reads the message as sent",
    () => jsonResponse({ ...NEW_MESSAGE, status: "sent" }),
  ],
])(
  "keeps the open composer and the reader's words when a later read %s",
  async (_, later) => {
    const user = userEvent.setup();
    let reads = 0;
    const sent = mount([NEW_MESSAGE], [], {
      [`GET /scheduled-sends/${NEW_MESSAGE.id}`]: () => {
        reads += 1;
        return reads === 1 ? jsonResponse(NEW_MESSAGE) : later();
      },
    });
    await user.click(
      await screen.findByRole("button", { name: "Edit: The renewal quote" }),
    );
    const subject = await screen.findByLabelText("Subject");
    await user.type(subject, ", revised");
    await act(() =>
      client.invalidateQueries({ queryKey: ["scheduled-sends"] }),
    );
    await waitFor(() => expect(reads).toBe(2));
    await waitFor(() =>
      expect(
        client.getQueryState(["scheduled-sends", NEW_MESSAGE.id])?.fetchStatus,
      ).toBe("idle"),
    );
    await act(async () => {});
    expect(screen.getByLabelText("Subject")).toHaveValue(
      "The renewal quote, revised",
    );
    expect(screen.queryByText("Message changed")).toBeNull();
    expect(
      sent.filter((call) => call.key === "GET /scheduled-sends"),
    ).toHaveLength(2);
  },
);

it("holds the loading state for a reply whose own read lands before its thread", async () => {
  const user = userEvent.setup();
  let answer: (response: Response) => void = () => {};
  mount([REPLY], [REPLY], {
    [`GET /activities/${REPLY.anchor_activity_id}`]: () =>
      new Promise<Response>((resolve) => {
        answer = resolve;
      }),
  });
  await user.click(
    await screen.findByRole("button", { name: "Edit: Re: the quote" }),
  );
  await waitFor(() =>
    expect(client.getQueryState(["scheduled-sends", REPLY.id])?.status).toBe(
      "success",
    ),
  );
  await act(async () => {});
  expect(
    within(screen.getByRole("dialog")).getByText("Loading message…"),
  ).toBeInTheDocument();
  answer(jsonResponse(THREAD_EMAIL));
  expect(await screen.findByLabelText("Subject")).toHaveValue("Re: the quote");
});

it("opens a closed and reopened edit from a fresh read of the message", async () => {
  const user = userEvent.setup();
  let reads = 0;
  mount([NEW_MESSAGE], [], {
    [`GET /scheduled-sends/${NEW_MESSAGE.id}`]: () => {
      reads += 1;
      return jsonResponse(
        reads === 1
          ? NEW_MESSAGE
          : { ...NEW_MESSAGE, subject: "The renewal quote, moved" },
      );
    },
  });
  const edit = await screen.findByRole("button", {
    name: "Edit: The renewal quote",
  });
  await user.click(edit);
  expect(await screen.findByLabelText("Subject")).toHaveValue(
    "The renewal quote",
  );
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Close" }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await user.click(edit);
  expect(await screen.findByLabelText("Subject")).toHaveValue(
    "The renewal quote, moved",
  );
});

it("keeps a reply waiting on its thread when a later read of the message fails", async () => {
  const user = userEvent.setup();
  let answer: (response: Response) => void = () => {};
  let reads = 0;
  mount([REPLY], [], {
    [`GET /scheduled-sends/${REPLY.id}`]: () => {
      reads += 1;
      return reads === 1
        ? jsonResponse(REPLY)
        : new Response(
            JSON.stringify({ type: "about:blank", title: "Down", status: 500 }),
            {
              status: 500,
              headers: { "Content-Type": "application/problem+json" },
            },
          );
    },
    [`GET /activities/${REPLY.anchor_activity_id}`]: () =>
      new Promise<Response>((resolve) => {
        answer = resolve;
      }),
  });
  await user.click(
    await screen.findByRole("button", { name: "Edit: Re: the quote" }),
  );
  await waitFor(() =>
    expect(client.getQueryState(["scheduled-sends", REPLY.id])?.status).toBe(
      "success",
    ),
  );
  await act(() =>
    client.refetchQueries({ queryKey: ["scheduled-sends", REPLY.id] }),
  );
  await waitFor(() =>
    expect(client.getQueryState(["scheduled-sends", REPLY.id])?.status).toBe(
      "error",
    ),
  );
  await act(async () => {});
  const waiting = within(screen.getByRole("dialog"));
  expect(waiting.getByText("Loading message…")).toBeInTheDocument();
  expect(
    screen.queryByText("Could not load this view. Reload the page."),
  ).toBeNull();
  answer(jsonResponse(THREAD_EMAIL));
  expect(await screen.findByLabelText("Subject")).toHaveValue("Re: the quote");
});
