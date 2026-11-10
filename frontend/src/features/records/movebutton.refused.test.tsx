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

function mount(move: PerformableMove) {
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
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <MoveButton dealId={DEAL} move={move} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return writes;
}

it("draws a refused task move with the server's reason and writes no task", async () => {
  const reason = "You do not have permission to add tasks";
  const writes = mount({
    action: "create_task",
    arguments: { subject: "Send the revised quote" },
    blocked_reason: reason,
  });
  const button = await screen.findByRole("button", {
    name: "Create follow-up",
  });
  expect(button).toBeDisabled();
  expect(button).toHaveAccessibleDescription(reason);
  await userEvent.click(button);
  expect(writes).toEqual([]);
});

it("draws a refused follow-up with the server's reason", async () => {
  const reason = "You do not have permission to send email";
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
