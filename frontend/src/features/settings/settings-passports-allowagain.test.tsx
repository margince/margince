/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { PassportCard } from "./settings-passports";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A revoked passport a chat app minted keeps its chat user out until the owner
// releases it. One with no app binding offers nothing to release.
function chatPassportsBackend(released: string[]) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const method = input instanceof Request ? input.method : "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["rep"],
        teams: [],
      });
    }
    if (url.endsWith("/app-binding") && method === "DELETE") {
      released.push(url.split("/passports/")[1].replace("/app-binding", ""));
      return new Response(null, { status: 204 });
    }
    if (url.includes("/passports")) {
      const bound = (id: string) => !released.includes(id);
      return jsonResponse({
        data: [
          {
            id: "pp-live",
            label: "Scout",
            scopes: ["read"],
            created_at: "2026-07-01T08:00:00Z",
            expires_at: null,
            revoked_at: null,
          },
          {
            id: "pp-chat",
            label: "Dispact",
            scopes: ["read"],
            created_at: "2026-06-01T08:00:00Z",
            expires_at: null,
            revoked_at: "2026-07-02T08:00:00Z",
            app_bound: bound("pp-chat"),
          },
          {
            id: "pp-old",
            label: "Retired",
            scopes: ["read"],
            created_at: "2026-05-01T08:00:00Z",
            expires_at: null,
            revoked_at: "2026-07-02T08:00:00Z",
            app_bound: false,
          },
        ],
        page: { next_cursor: null, has_more: false },
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

describe("PassportCard allow again", () => {
  it("releases a revoked chat passport and offers nothing on one with no chat user", async () => {
    const user = userEvent.setup();
    const released: string[] = [];
    vi.stubGlobal("fetch", chatPassportsBackend(released));
    render(<PassportCard />);
    await user.click(await screen.findByRole("button", { name: /revoked/i }));
    expect(
      screen.queryByRole("button", { name: "Allow Retired again" }),
    ).toBeNull();
    await user.click(
      await screen.findByRole("button", { name: "Allow Dispact again" }),
    );
    await waitFor(() => expect(released).toEqual(["pp-chat"]));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Allow Dispact again" }),
      ).toBeNull(),
    );
  });
});
