/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  jsonResponse,
  ROSTER,
  render,
  rowFor,
  rowMenu,
} from "./users-admin.testkit";
import { MAIL_POLL_MS, rosterPollInterval, type User } from "./users-members";

// An invited member whose invitation mail the relay refused: the roster says
// so on the row and offers a resend until one goes out.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const REFUSED: Omit<User, "timezone"> = {
  id: "u-refused",
  email: "refused@acme.test",
  display_name: "Rhea Refused",
  status: "invited",
  is_agent: false,
  roles: ["rep"],
  allowed_actions: ["issue_password_link", "resend_invite", "deactivate"],
  invite_email_state: "failed",
  team_ids: [],
  created_at: "2026-11-03T08:59:00Z",
};

// The roster as the server answers it before and after the resend.
function invitingBackend(calls: { method: string; url: string }[]) {
  const rest = backend(calls);
  let resent = false;
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const path = new URL(req.url).pathname;
    if (path.endsWith("/v1/users") && req.method === "GET") {
      const refused = resent
        ? {
            ...REFUSED,
            invite_email_state: "sent",
            allowed_actions: ["issue_password_link", "deactivate"],
          }
        : REFUSED;
      return jsonResponse({ ...ROSTER, data: [...ROSTER.data, refused] });
    }
    if (path.endsWith("/resend-invite")) {
      calls.push({ method: req.method, url: req.url });
      resent = true;
      return jsonResponse({
        ...REFUSED,
        invite_email: { sendable: true, queued: true },
      });
    }
    return rest(input, init);
  });
}

describe("UsersAdminCard invitation mail", () => {
  it("marks a refused invitation and resends it from the row", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string }[] = [];
    vi.stubGlobal("fetch", invitingBackend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Rhea Refused")).toBeTruthy());

    const row = rowFor("Rhea Refused");
    expect(
      within(row).getByText(en["users.status.inviteMailFailed"]),
    ).toBeTruthy();

    const verbs = await rowMenu(user, "Rhea Refused");
    await user.click(verbs.getByText(en["users.resendInvite"]));

    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.method === "POST" &&
            c.url.includes("/users/u-refused/resend-invite"),
        ),
      ).toBe(true),
    );
    await waitFor(() =>
      expect(
        within(rowFor("Rhea Refused")).queryByText(
          en["users.status.inviteMailFailed"],
        ),
      ).toBeNull(),
    );
    expect(
      within(rowFor("Rhea Refused")).getByText(en["users.status.invited"]),
    ).toBeTruthy();
  });

  it("reads the roster again until a mail on its way is answered", async () => {
    vi.useFakeTimers({ toFake: ["setInterval"] });
    let reads = 0;
    // The relay answers once the test says so, whatever reads came before.
    let answered = false;
    const rest = backend([]);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (
          new URL(req.url).pathname.endsWith("/v1/users") &&
          req.method === "GET"
        ) {
          reads++;
          const pending = { ...REFUSED, invite_email_state: "sending" };
          return jsonResponse({
            ...ROSTER,
            data: [...ROSTER.data, answered ? REFUSED : pending],
          });
        }
        return rest(input, init);
      }),
    );
    try {
      render(<UsersAdminCard />);
      await waitFor(() =>
        expect(screen.getByText("Rhea Refused")).toBeTruthy(),
      );
      expect(
        screen.queryByText(en["users.status.inviteMailFailed"]),
      ).toBeNull();

      answered = true;
      vi.advanceTimersByTime(MAIL_POLL_MS);
      await waitFor(() =>
        expect(
          within(rowFor("Rhea Refused")).getByText(
            en["users.status.inviteMailFailed"],
          ),
        ).toBeTruthy(),
      );
      expect(reads).toBeGreaterThan(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("polls only while a mail is on its way", () => {
    const member = (state: User["invite_email_state"]) => ({
      ...REFUSED,
      invite_email_state: state,
    });
    expect(rosterPollInterval([member("sending")])).toBe(MAIL_POLL_MS);
    expect(rosterPollInterval([member("failed")])).toBe(false);
    expect(rosterPollInterval(undefined)).toBe(false);
  });

  it("marks a plain invitation as invited only", async () => {
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());
    expect(screen.queryByText(en["users.status.inviteMailFailed"])).toBeNull();
  });
});
