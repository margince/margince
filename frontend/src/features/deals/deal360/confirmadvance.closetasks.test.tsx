// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { BulkUndoProvider } from "../../../components/bulkchange";
import { pickOption } from "../../../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../../../design-system/toast";
import { LocaleProvider } from "../../../i18n";
import { ConfirmAdvanceModal } from "./confirmadvance";

// Closing a deal with open tasks asks what happens to them. Complete all runs
// one bulk change over the listed tasks, once the close has landed. Keep and a
// refused close complete nothing.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const LOST: components["schemas"]["Stage"] = {
  id: "stage-lost",
  pipeline_id: "pl-1",
  name: "Closed lost",
  position: 4,
  semantic: "lost",
  win_probability: 0,
};

function task(id: string, subject: string, done = false) {
  return {
    id,
    kind: "task",
    subject,
    is_done: done,
    version: 2,
    occurred_at: "2026-10-01T09:00:00Z",
  };
}

function json(body: unknown) {
  return Response.json(body);
}

type Sent = { path: string; body: unknown };
type BulkBody = { items: { id: string; version: number }[] };

const OPEN_TASKS = [
  task("t-invoice", "Send the final invoice"),
  task("t-device", "Return the loan device"),
  task("t-done", "Already handled", true),
  { ...task("n-kickoff", "Kickoff notes"), kind: "note", is_done: undefined },
];

type Server = Readonly<{
  activities?: () => Response | Promise<Response>;
  preview?: (body: BulkBody) => unknown;
}>;

function previewOf(body: BulkBody) {
  return {
    record_type: "worklist_item",
    verb: "complete",
    count: body.items.length,
    affected: body.items.map((item) => item.id),
    sample: [],
    excluded: [],
    requires_confirmation: body.items.length > 10,
    confirm_token: `token-${body.items.length}`,
  };
}

// The deal's task list (two open, one done, one note by default) and the bulk
// calls, each execute answering as a batch of its own.
// The one reason the close picker offers here.
const LOST_TO_A_RIVAL = {
  id: "reason-competitor",
  key: "competitor",
  label: "Chose a competitor",
  active: true,
  sort_order: 10,
  system: true,
  version: 1,
  created_at: "2026-10-01T08:00:00Z",
  updated_at: "2026-10-01T08:00:00Z",
};

// Gives the loss its reason, the one answer a lost close always needs.
async function giveReason(user: ReturnType<typeof userEvent.setup>) {
  await pickOption(
    user,
    screen.getByRole("combobox", { name: "Why was it lost?" }),
    "Chose a competitor",
  );
}

function stubServer(server: Server = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = new URL(input.url, "https://test.local");
      // An undo's preview carries no body at all.
      const text = input.method === "POST" ? await input.clone().text() : "";
      const body = text === "" ? undefined : JSON.parse(text);
      if (input.method === "POST") {
        sent.push({ path: url.pathname, body });
      }
      if (url.pathname === "/v1/loss-reasons") {
        return json({ data: [LOST_TO_A_RIVAL] });
      }
      if (url.pathname === "/v1/activities") {
        return (
          server.activities?.() ??
          json({ data: OPEN_TASKS, page: { has_more: false } })
        );
      }
      if (url.pathname === "/v1/bulk/preview") {
        return json((server.preview ?? previewOf)(body));
      }
      if (url.pathname === "/v1/bulk/execute") {
        const items = (body as BulkBody).items;
        return json({
          batch_id: `batch-${items[0]?.id}`,
          changed: items.length,
          skipped: [],
        });
      }
      return json({ data: [], page: { has_more: false } });
    }),
  );
  return sent;
}

function renderClose(onConfirm: () => Promise<unknown>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <BulkUndoProvider>
            <ConfirmAdvanceModal
              pending={{ dealId: "deal-1", version: 3, toStage: LOST }}
              onClose={() => {}}
              onConfirm={onConfirm}
            />
          </BulkUndoProvider>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function answer(
  user: ReturnType<typeof userEvent.setup>,
  choice: string,
) {
  const dialog = within(screen.getByRole("dialog"));
  await giveReason(user);
  await user.click(dialog.getByRole("radio", { name: new RegExp(choice) }));
  await user.click(dialog.getByRole("button", { name: "Confirm" }));
}

it("lists the deal's open tasks and asks before it closes", async () => {
  stubServer();
  renderClose(async () => ({ id: "deal-1", version: 4 }));
  const list = await screen.findByRole("list", {
    name: "Open tasks on this deal",
  });
  expect(
    within(list)
      .getAllByRole("listitem")
      .map((item) => item.textContent),
  ).toEqual(["Send the final invoice", "Return the loan device"]);
  expect(
    screen.getByRole("group", {
      name: "What happens to the 2 open tasks on this deal?",
    }),
  ).toBeInTheDocument();
  expect(screen.getByRole("radio", { name: /Complete all/ })).not.toBeChecked();
  expect(screen.getByRole("radio", { name: /Keep/ })).not.toBeChecked();

  // A reason alone does not close it: the tasks still need an answer.
  const user = userEvent.setup();
  await giveReason(user);
  expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
});

it("completes exactly the listed tasks after the close lands, with undo", async () => {
  const sent = stubServer();
  const user = userEvent.setup();
  const order: string[] = [];
  renderClose(async () => {
    order.push("advance");
    return { id: "deal-1", version: 4 };
  });
  await screen.findByRole("list", { name: "Open tasks on this deal" });
  await answer(user, "Complete all");

  await waitFor(() =>
    expect(sent.map((call) => call.path)).toEqual([
      "/v1/bulk/preview",
      "/v1/bulk/execute",
    ]),
  );
  expect(order).toEqual(["advance"]);
  expect(sent[1]?.body).toMatchObject({
    record_type: "worklist_item",
    verb: "complete",
    confirm_token: "token-2",
    items: [
      { id: "t-invoice", version: 2 },
      { id: "t-device", version: 2 },
    ],
  });
  expect(
    await screen.findByRole("button", { name: "Undo" }),
  ).toBeInTheDocument();
});

it("keeps every task open when the reader picks Keep", async () => {
  const sent = stubServer();
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => ({ id: "deal-1", version: 4 }));
  renderClose(onConfirm);
  await screen.findByRole("list", { name: "Open tasks on this deal" });
  await answer(user, "Keep");

  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  expect(sent).toEqual([]);
});

it("completes nothing when the close is refused", async () => {
  const sent = stubServer();
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => {
    throw new Error("version conflict");
  });
  renderClose(onConfirm);
  await screen.findByRole("list", { name: "Open tasks on this deal" });
  await answer(user, "Complete all");

  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  expect(sent).toEqual([]);
});

it("holds Confirm while the deal's tasks are still loading", async () => {
  let answerRead: (response: Response) => void = () => {};
  stubServer({
    activities: () =>
      new Promise((resolve) => {
        answerRead = resolve;
      }),
  });
  const user = userEvent.setup();
  renderClose(async () => ({ id: "deal-1", version: 4 }));
  await giveReason(user);
  const confirm = screen.getByRole("button", { name: "Confirm" });
  expect(confirm).toBeDisabled();
  expect(
    screen.getByText("Loading the deal’s open tasks…"),
  ).toBeInTheDocument();

  answerRead(json({ data: OPEN_TASKS, page: { has_more: false } }));
  await screen.findByRole("list", { name: "Open tasks on this deal" });
  expect(screen.queryByText("Loading the deal’s open tasks…")).toBeNull();
});

it("lets the deal close after a failed read and keeps every task", async () => {
  const sent = stubServer({
    activities: () =>
      Response.json(
        { code: "internal", title: "Internal Server Error", status: 500 },
        { status: 500 },
      ),
  });
  const user = userEvent.setup();
  const onConfirm = vi.fn(async () => ({ id: "deal-1", version: 4 }));
  renderClose(onConfirm);
  await screen.findByText("Closing keeps every task on this deal open.");
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(screen.queryByRole("radio")).toBeNull();

  await giveReason(user);
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  expect(sent).toEqual([]);
});

it("says so when Complete all could complete none of the tasks", async () => {
  const sent = stubServer({
    preview: (body) => ({
      ...previewOf(body),
      count: 0,
      affected: [],
      confirm_token: undefined,
      excluded: body.items.map((item) => ({
        id: item.id,
        reason: "changed_since_preview",
      })),
    }),
  });
  const user = userEvent.setup();
  renderClose(async () => ({ id: "deal-1", version: 4 }));
  await screen.findByRole("list", { name: "Open tasks on this deal" });
  await answer(user, "Complete all");

  const note = await screen.findByText(/No task could be completed\./);
  expect(note.textContent).not.toBe("No task could be completed.");
  expect(sent.map((call) => call.path)).toEqual(["/v1/bulk/preview"]);
});

it("completes more tasks than one bulk change holds in batches of 500", async () => {
  const many = Array.from({ length: 501 }, (_, n) =>
    task(`t-${n}`, `Task ${n}`),
  );
  const sent = stubServer({
    activities: () => json({ data: many, page: { has_more: false } }),
  });
  const user = userEvent.setup();
  renderClose(async () => ({ id: "deal-1", version: 4 }));
  await screen.findByRole("list", { name: "Open tasks on this deal" });
  await answer(user, "Complete all");

  await waitFor(() => expect(sent).toHaveLength(4));
  const executed = sent
    .filter((call) => call.path === "/v1/bulk/execute")
    .map((call) => (call.body as BulkBody).items.map((item) => item.id));
  expect(executed.map((ids) => ids.length)).toEqual([500, 1]);
  expect(executed.flat()).toEqual(many.map((row) => row.id));
  // One note for the whole change; its Undo walks the batches in order.
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  await waitFor(() =>
    expect(sent.at(-1)?.path).toBe("/v1/bulk/batch-t-0/undo/preview"),
  );
});
