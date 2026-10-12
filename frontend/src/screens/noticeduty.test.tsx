/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { en } from "../i18n/en";
import { day, jsonResponse, renderWorklist, row } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CONTACT = "01a05500-0000-7000-8000-0000000000aa";
const OWED = row({
  id: "01a05500-0000-7000-8000-0000000000cc",
  source: "notice_case",
  kind: "art14",
  due_at: "2026-02-28T09:00:00Z",
  owner: { kind: "unassigned" },
  acquisition: {
    kind: "mailbox_history",
    occurred_at: null,
    captured_at: "2026-01-28T09:00:00Z",
    captured_by: "connector:gmail",
    captured_by_name: null,
  },
  subject: { type: "contact", id: CONTACT, label: "Steve Leichsenring" },
});

function dutyDay(duty = OWED) {
  return day({
    queue: [duty],
    summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
  });
}

// The server a writer meets: the queue with one duty, a /me that may update
// contacts, and every write recorded.
function stubDuty(duty = OWED, seat: "full" | "read" = "full") {
  const writes: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = String(request ? request.url : input);
      const path = url.split("?")[0];
      const method = request?.method ?? init?.method ?? "GET";
      if (path.endsWith("/worklist")) {
        return jsonResponse(dutyDay(duty));
      }
      if (path.endsWith(`/privacy/notice-cases/${OWED.id}`)) {
        return jsonResponse({
          id: OWED.id,
          contact_id: CONTACT,
          rule: "art14",
          due_at: "2026-02-28T09:00:00Z",
          state: "open",
          owner_user_id: null,
          attempts: 0,
          created_at: "2026-01-28T09:00:00Z",
        });
      }
      if (path.endsWith("/me")) {
        return jsonResponse(
          meFixture({
            seat,
            allow: { contact: ["read", "update"], privacy_request: ["read"] },
          }),
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

  it("says what the deadline rests on: how, when, who recorded it, and the rule", async () => {
    stubDuty();
    renderWorklist();

    const recorded = await screen.findByText(/Via gmail$/);
    const facts = recorded.closest("dl");
    if (!(facts instanceof HTMLElement)) throw new Error("no fact list");
    expect(
      within(facts).getByText(en["notice.acq.mailboxHistory"]),
    ).toBeTruthy();
    expect(within(facts).getByText(en["noticeDuty.dateUnknown"])).toBeTruthy();
    // Who claimed it comes from the case, not from whose queue the row sits in.
    expect(
      await within(facts).findByText(en["notice.unassigned"]),
    ).toBeTruthy();
    expect(within(facts).getByText("Art. 14 notice")).toBeTruthy();
    expect(within(facts).getByText(en["notice.ruleHint.art14"])).toBeTruthy();
  });

  it("says a duty with no evidence row has none, rather than a bare date", async () => {
    stubDuty({ ...OWED, acquisition: null });
    renderWorklist();

    expect(await screen.findByText(en["notice.noAcquisition"])).toBeTruthy();
    expect(screen.queryByText(en["noticeDuty.obtainedOn"])).toBeNull();
  });

  it("offers a read seat no verb, even with the contact grant", async () => {
    stubDuty(OWED, "read");
    renderWorklist();

    // The case read answers only once /me has, so the seat is known here.
    expect(await screen.findByText(en["notice.unassigned"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["noticeDuty.sendNotice"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["noticeDuty.end"] }),
    ).toBeNull();
  });
});
