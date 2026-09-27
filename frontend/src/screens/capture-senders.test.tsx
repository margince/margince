/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { CaptureSendersCard } from "./capture-senders";
import { jsonResponse, render } from "./settings.testkit";

function stubRoutes(waiting: unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const path = url.pathname.replace(/^\/v1/, "");
      if (path === "/capture/senders") return jsonResponse({ data: [] });
      if (path === "/capture/contacts-awaiting-decision")
        return jsonResponse({ data: waiting });
      if (path === "/me") return jsonResponse(meFixture({ roles: ["rep"] }));
      return jsonResponse({});
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The contact with no sender entry is the one the senders list cannot show,
// and the one the admin card counts for this mailbox all the same.
it("lists a waiting contact whose sender never reached the senders list", async () => {
  stubRoutes([
    {
      contact_id: "0190b7a2-0000-7000-8000-000000000012",
      display_name: "Marta Ortiz",
      emails: ["m.ortiz@steireif.de"],
      captured_at: "2026-09-10T14:40:00Z",
    },
  ]);

  render(<CaptureSendersCard />);

  expect(await screen.findByText("Waiting on a decision")).toBeInTheDocument();
  expect(await screen.findByText("Marta Ortiz")).toBeInTheDocument();
  expect(screen.getByText("m.ortiz@steireif.de")).toBeInTheDocument();
  expect(screen.getByText("No senders yet")).toBeInTheDocument();
});

it("draws no waiting group when nothing waits", async () => {
  stubRoutes([]);

  const { client } = render(<CaptureSendersCard />);

  expect(await screen.findByText("No senders yet")).toBeInTheDocument();
  // Settled first, so the absence below is the empty answer and not a read
  // still in flight.
  await waitFor(() =>
    expect(
      client.getQueryState(["capture-contacts-awaiting-decision"])?.status,
    ).toBe("success"),
  );
  expect(screen.queryByText("Waiting on a decision")).not.toBeInTheDocument();
});
