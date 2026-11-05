// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { settingsReach } from "../../lib/settingscatalog";
import { STORAGE_KEYS, writeStored } from "../../lib/storage";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { newClient, renderWith } from "../../testing/shellharness";
import { SettingsHome } from "./settingshome";

// The "Your setup" card at the top of the settings home. It holds the steps the
// top bar reminds a reader of. It stays when that reminder is hidden, and goes
// once every step is done.

const VOICE_WRITER: GrantSpec = { voice_profile: ["read", "create", "update"] };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function connection(provider: "gmail" | "gcal") {
  return {
    id: `00000000-0000-4000-8000-${provider.padStart(12, "0")}`,
    provider,
    status: "connected",
    scopes:
      provider === "gcal"
        ? ["https://www.googleapis.com/auth/calendar.events"]
        : [],
    account_label: "ada@example.test",
  };
}

function serve({ finished }: Readonly<{ finished: boolean }>) {
  const answers: Readonly<Record<string, () => Response>> = {
    "/connectors": () =>
      jsonResponse({
        data: finished
          ? [connection("gmail"), connection("gcal")]
          : [connection("gmail")],
        providers: ["gmail", "graph", "imap"].map((provider) => ({
          provider,
          reason: "ready",
        })),
      }),
    "/voice-profiles": () =>
      jsonResponse({ data: finished ? [{ profile_version: 1 }] : [] }),
    "/scheduling/profile": () =>
      jsonResponse({
        enabled: finished,
        provider: "",
        calendar_id: "",
        duration_minutes: 30,
        notice_minutes: 240,
        horizon_days: 30,
        buffer_minutes: 0,
        title: "Intro call",
        location: "",
      }),
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const route = Object.keys(answers).find((end) => path.endsWith(end));
      return route ? answers[route]() : jsonResponse({}, 404);
    }),
  );
}

function mountHome() {
  const snapshot = meFixture({ allow: VOICE_WRITER });
  const client = newClient();
  client.setQueryData(["me"], snapshot);
  renderWith(client, <SettingsHome reach={settingsReach(snapshot)} />);
  return client;
}

async function card(): Promise<HTMLElement> {
  const heading = await screen.findByRole("heading", { name: "Your setup" });
  const panel = heading.closest("section, .panel");
  if (!(panel instanceof HTMLElement)) {
    throw new Error("the Your setup heading stands in no panel");
  }
  return panel;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("Your setup card", () => {
  it("leads the home with the reader's steps and how many are done", async () => {
    serve({ finished: false });
    mountHome();

    const setup = await card();
    const headings = screen.getAllByRole("heading", { level: 2 });
    expect(headings[0].textContent).toBe("Your setup");
    expect(within(setup).getByText("Only you")).not.toBeNull();
    expect(within(setup).getByText("1 of 4 done")).not.toBeNull();
    expect(within(setup).getAllByRole("listitem")).toHaveLength(4);
    expect(
      within(setup).getByRole("button", { name: "Connect calendar" }),
    ).not.toBeNull();
  });

  it("stays when the reader hid the top bar reminder", async () => {
    writeStored(STORAGE_KEYS.setupReminderHidden, "1");
    serve({ finished: false });
    mountHome();

    expect(within(await card()).getAllByRole("listitem")).toHaveLength(4);
  });

  it("is gone once every step is done, and the steps' pages stay listed", async () => {
    serve({ finished: true });
    const client = mountHome();

    // Every setup read answered, so an absent card is the decision, not a wait.
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledTimes(3);
      expect(client.isFetching()).toBe(0);
    });
    const directory = screen.getByRole("link", { name: /Connections/ });
    expect(directory.getAttribute("href")).toBe("/#/settings/connections");
    expect(screen.queryByRole("heading", { name: "Your setup" })).toBeNull();
    expect(screen.queryByText("Setup is complete")).toBeNull();
  });
});
