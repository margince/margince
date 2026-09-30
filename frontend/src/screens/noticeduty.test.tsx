/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { day, jsonResponse, renderWorklist, row } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CONTACT = "01a05500-0000-7000-8000-0000000000aa";
const DUTY = day({
  queue: [
    row({
      id: "01a05500-0000-7000-8000-0000000000cc",
      source: "notice_case",
      kind: "art14",
      subject: { type: "contact", id: CONTACT, label: "Steve Leichsenring" },
    }),
  ],
  summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
});

// The server a writer meets: the queue with one duty, a /me that may update
// contacts, and every write recorded.
function stubDuty() {
  const writes: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const path = url.split("?")[0];
      const method = request?.method ?? init?.method ?? "GET";
      if (path.endsWith("/worklist")) {
        return jsonResponse(DUTY);
      }
      if (path.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { contact: ["read", "update"] } }),
        );
      }
      if (method === "POST") {
        writes.push(path);
        return jsonResponse({
          delivered_to: "steve@kps.test",
          expires_at: "2026-10-27T00:00:00Z",
          queued: true,
          sendable: true,
        });
      }
      return jsonResponse({ data: [] });
    }),
  );
  return writes;
}

describe("a privacy-notice duty on the Focus card", () => {
  it("says what the duty is and sends the notice from the card's own pane", async () => {
    const writes = stubDuty();
    renderWorklist();

    expect(
      await screen.findByText("Privacy notice owed (GDPR Art. 14)"),
    ).toBeTruthy();
    expect(screen.getByText(/did not get them from the contact/)).toBeTruthy();
    expect(
      await screen.findByRole("button", {
        name: "Ask them to confirm their details",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "End the duty…" })).toBeTruthy();

    await userEvent.click(
      screen.getByRole("button", { name: "Send privacy notice" }),
    );
    await waitFor(() =>
      expect(
        writes.some((path) =>
          path.endsWith(`/contacts/${CONTACT}/consent/privacy-notice`),
        ),
      ).toBe(true),
    );
    expect(await screen.findByText(/Sent to steve@kps.test/)).toBeTruthy();
  });
});
