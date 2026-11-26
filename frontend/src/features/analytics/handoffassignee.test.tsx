/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { WaitingRow } from "./analytics.outcomes.sdr";
import { HandoffAssigneeModal } from "./handoffassignee";

const colleague = "10000000-0000-4000-8000-000000000002";
const handoff = "10000000-0000-4000-8000-000000000001";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      "content-type":
        status === 200 ? "application/json" : "application/problem+json",
    },
  });
}

function mount(children?: ReactNode) {
  const closed = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      {children ?? <HandoffAssigneeModal id={handoff} onClose={closed} />}
    </QueryClientProvider>,
  );
  return closed;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("keeps the chosen receiver after refusal and retries with the current version", async () => {
  const user = userEvent.setup();
  const writes: unknown[] = [];
  let version = 3;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("expected an API request");
      if (input.method === "PATCH") {
        writes.push(await input.json());
        if (writes.length === 1) {
          version = 4;
          return json(
            {
              status: 422,
              title: "Choose another receiver",
              detail: "This colleague cannot work the prospect.",
            },
            422,
          );
        }
        return json({
          id: handoff,
          version: 5,
          status: "submitted",
          assigned_to: colleague,
          can_reassign: true,
        });
      }
      if (new URL(input.url).pathname.endsWith("/users"))
        return json({
          data: [
            {
              id: colleague,
              display_name: "Dana Example",
              status: "active",
              is_agent: false,
              can_own_records: true,
            },
          ],
          page: { has_more: false, next_cursor: null },
        });
      return json({
        id: handoff,
        version,
        status: "submitted",
        can_reassign: true,
      });
    }),
  );
  const closed = mount();
  const confirm = () =>
    screen.getByRole("button", { name: "Reassign handoff" });
  expect(confirm()).toBeDisabled();
  await waitFor(() => expect(screen.getByRole("combobox")).not.toBeDisabled());
  await user.click(screen.getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: "Dana Example" }));
  await waitFor(() => expect(confirm()).not.toBeDisabled());
  await user.click(confirm());
  await waitFor(() => expect(writes).toHaveLength(1));
  await waitFor(() => expect(confirm()).not.toBeDisabled());
  expect(closed).not.toHaveBeenCalled();
  expect(screen.getByRole("combobox")).toHaveTextContent("Dana Example");
  await user.click(confirm());
  await waitFor(() => expect(closed).toHaveBeenCalledOnce());
  expect(writes).toEqual([
    { assigned_to: colleague, version: 3 },
    { assigned_to: colleague, version: 4 },
  ]);
});

it("prevents reassignment once the server says the handoff was answered", async () => {
  const user = userEvent.setup();
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    if (!(input instanceof Request)) throw new Error("expected an API request");
    if (new URL(input.url).pathname.endsWith("/users"))
      return json({ data: [], page: { has_more: false } });
    return json({
      id: handoff,
      version: 2,
      status: "accepted",
      can_reassign: false,
    });
  });
  vi.stubGlobal("fetch", fetch);
  mount();
  await screen.findByText(/Accepted handoffs keep/);
  await user.click(screen.getByRole("button", { name: "Reassign handoff" }));
  expect(
    screen.getByRole("button", { name: "Reassign handoff" }),
  ).toBeDisabled();
  expect(
    fetch.mock.calls.every(
      ([input]) => input instanceof Request && input.method === "GET",
    ),
  ).toBe(true);
});

it("opens a fresh assignment from a waiting row without member administration", async () => {
  const user = userEvent.setup();
  const reads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      if (!(input instanceof Request))
        throw new Error("expected an API request");
      const path = new URL(input.url).pathname;
      reads.push(path);
      if (path.endsWith("/me"))
        return json(
          meFixture({
            roles: ["rep"],
            allow: { lead: ["read", "update"], contact: ["read", "update"] },
          }),
        );
      if (path.endsWith("/users"))
        return json({ data: [], page: { has_more: false } });
      return json({
        id: handoff,
        version: 7,
        status: "submitted",
        can_reassign: true,
      });
    }),
  );
  mount(
    <WaitingRow
      row={{
        id: handoff,
        waiting_since: "2026-09-01T09:00:00Z",
        resubmitted: false,
        restricted: false,
        subject: { type: "contact", id: colleague, label: "Dana Buyer" },
        recipient: { id: "former", label: "Robin Former", active: false },
      }}
      asOf={Date.parse("2026-09-04T09:00:00Z")}
      locale="en"
    />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Reassign handoff" }),
  );
  await screen.findByRole("dialog");
  await waitFor(() =>
    expect(reads).toContain(`/v1/sdr-handoffs/${handoff}/assignee`),
  );
  expect(screen.getByRole("combobox")).toBeEnabled();
});
