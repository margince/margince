/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { RequestsPanel } from "../settings/settings.privacy";
import { dueAtOf, earlierDueTooLate } from "./privacy.deadline";

// The deadline of a manual subject request. The server computes it and the form
// shows it. The form sends a date only when the officer chose an earlier one.

const ZONE = "America/New_York";
const COMPUTED = { due_at: "2026-08-15T14:00:00Z", rule: "gdpr" };

type Sent = { key: string; url: string; body: unknown };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stub(): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input), "https://t");
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      const body: unknown =
        method === "GET"
          ? null
          : request
            ? await request.json()
            : JSON.parse(String(init?.body));
      sent.push({ key, url: url.pathname + url.search, body });
      switch (key) {
        case "GET /me":
          return json(
            meFixture({
              roles: ["admin"],
              allow: {
                privacy_request: ["read", "update"],
                contact: ["update"],
              },
            }),
          );
        case "GET /data-subject-requests/deadline":
          return json(COMPUTED);
        case "POST /data-subject-requests":
          return json(
            {
              id: "d1",
              kind: "access",
              subject_ref: "anna@acme.test",
              status: "open",
              due_at: COMPUTED.due_at,
              due_rule: "gdpr",
              due_overridden: false,
              created_at: "2026-07-15T00:00:00Z",
            },
            201,
          );
      }
      return json({ data: [], page: { next_cursor: null, has_more: false } });
    }),
  );
  return sent;
}

async function openAccessForm(user: ReturnType<typeof userEvent.setup>) {
  rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <RequestsPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(await screen.findByRole("button", { name: /new request/i }));
  await pickOption(user, screen.getByLabelText(/kind/i), "Access");
  await user.type(
    screen.getByLabelText(/subject reference/i),
    "anna@acme.test",
  );
  await screen.findByText(/GDPR, one calendar month/);
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
  vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
    timeZone: ZONE,
  } as Intl.ResolvedDateTimeFormatOptions);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("the deadline field", () => {
  it("shows the computed deadline and its rule, and opens without a date of its own", async () => {
    stub();
    const user = userEvent.setup();
    await openAccessForm(user);
    expect(screen.getByRole("button", { name: /open request/i })).toBeEnabled();
  });

  it("refuses an earlier deadline that is not earlier", async () => {
    stub();
    const user = userEvent.setup();
    await openAccessForm(user);
    await user.type(
      screen.getByRole("textbox", { name: /earlier deadline/i }),
      "20/08/2026",
    );
    await user.tab();
    expect(screen.getByText(/choose a date before/i)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /open request/i }),
    ).toBeDisabled();
  });

  it("sends the day a late request arrived, and asks the deadline from that day", async () => {
    const sent = stub();
    const user = userEvent.setup();
    await openAccessForm(user);
    await user.type(
      screen.getByRole("textbox", { name: /received on/i }),
      "01/07/2026",
    );
    await user.tab();
    await waitFor(() =>
      expect(
        sent.some(
          (s) =>
            s.key === "GET /data-subject-requests/deadline" &&
            s.url.includes("received_at=2026-07-01T04%3A00%3A00.000Z"),
        ),
      ).toBe(true),
    );
    await user.click(screen.getByRole("button", { name: /open request/i }));
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "POST /data-subject-requests")?.body,
      ).toEqual({
        kind: "access",
        subject_ref: "anna@acme.test",
        received_at: "2026-07-01T04:00:00.000Z",
      }),
    );
  });
});

describe("dueAtOf", () => {
  const exact = { due_at: "2026-08-04T10:00:00Z", rule: "zz v1" };

  it("sends nothing when no earlier day was picked", () => {
    expect(dueAtOf({ receivedOn: "", earlierDue: "" }, exact, ZONE)).toBe(
      undefined,
    );
  });

  // The end of the deadline's own day is past an exact 72-hour deadline, and
  // the server would refuse it.
  it("sends nothing for the deadline's own day, so an hour-exact deadline is never passed", () => {
    expect(
      dueAtOf({ receivedOn: "", earlierDue: "2026-08-04" }, exact, ZONE),
    ).toBe(undefined);
    expect(
      earlierDueTooLate(
        { receivedOn: "", earlierDue: "2026-08-04" },
        exact,
        ZONE,
      ),
    ).toBe(false);
  });

  it("sends the end of an earlier day", () => {
    expect(
      dueAtOf({ receivedOn: "", earlierDue: "2026-08-02" }, exact, ZONE),
    ).toBe("2026-08-03T03:59:59.999Z");
  });
});
