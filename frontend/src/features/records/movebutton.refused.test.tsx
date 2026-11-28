// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A move the server refused this reader is drawn, refused, with the server's
// reason, and pressing it writes nothing.

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { sessionOptions } from "../../lib/session";
import { meFixture } from "../../testing/mefixture";
import { MoveButton, type PerformableMove } from "./movebutton";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const [DEAL, MEETING, CONTACT] = [
  "01a05500-0000-7000-8000-0000000000b1",
  "01a05500-0000-7000-8000-0000000000e1",
  "01a05500-0000-7000-8000-0000000000d1",
];

function mount(move: PerformableMove, client = new QueryClient()) {
  const writes: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      if (request && request.method !== "GET") writes.push(url);
      if (url.endsWith("/v1/me")) {
        return Response.json(meFixture({ allow: { activity: ["read"] } }));
      }
      return Response.json({ data: [] });
    }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <MoveButton dealId={DEAL} move={move} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return writes;
}

it("draws a refused task move with the server's reason and writes no task", async () => {
  const reason =
    "Your role can only read. Ask an administrator for another role.";
  const writes = mount({
    action: "create_task",
    arguments: { subject: "Send the revised quote" },
    blocked_reason: reason,
  });
  const button = await screen.findByRole("button", {
    name: englishCopy["deal360.createTask"],
  });
  expect(button).toBeDisabled();
  expect(button).toHaveAccessibleDescription(reason);
  await userEvent.click(button);
  expect(writes).toEqual([]);
});

it("draws a refused follow-up with the server's reason", async () => {
  const reason =
    "Your role can only read. Ask an administrator for another role.";
  mount({
    action: "draft_email",
    arguments: {
      activity_id: MEETING,
      links: [{ entity_type: "contact", entity_id: CONTACT }],
    },
    blocked_reason: reason,
  });
  const button = await screen.findByRole("button", {
    name: /draft follow-up/i,
  });
  expect(button).toBeDisabled();
  expect(button).toHaveAccessibleDescription(reason);
});

// The server marks every move it serves, so a move it did not refuse is
// offered. The client keeps no second answer that could hide it.
it("offers a task move the server did not refuse", async () => {
  // The session is answered before the first render, so a client-side grant
  // check would already refuse here.
  const client = new QueryClient();
  client.setQueryData(
    sessionOptions.queryKey,
    meFixture({ allow: { activity: ["read"] } }),
  );
  mount(
    {
      action: "create_task",
      arguments: { subject: "Send the revised quote" },
    },
    client,
  );
  const button = await screen.findByRole("button", {
    name: englishCopy["deal360.createTask"],
  });
  expect(button).toBeEnabled();
  expect(button).not.toHaveAccessibleDescription();
});

it("draws a refused bring-in-the-manager move with the server's reason and writes no request", async () => {
  const reason = "You do not have permission to change deals";
  const writes = mount({
    action: "bring_in_manager",
    arguments: { deal_id: DEAL, reasons: ["stuck"] },
    blocked_reason: reason,
  });
  const button = await screen.findByRole("button", {
    name: /bring in manager/i,
  });
  expect(button).toBeDisabled();
  expect(button).toHaveAccessibleDescription(reason);
  await userEvent.click(button);
  expect(writes).toEqual([]);
});
