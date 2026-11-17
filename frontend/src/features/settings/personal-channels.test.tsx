/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import { PersonalChannelsCard } from "./personal-channels";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function backend(connection: Record<string, unknown>) {
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input));
      if (url.pathname.endsWith("/personal-channels/connection")) {
        if (request?.method === "PATCH") {
          const body = (await request.json()) as { enabled: boolean };
          sent.push(body);
          return json({ ...connection, enabled: body.enabled });
        }
        return json(connection);
      }
      if (url.pathname.endsWith("/passports")) {
        return json({ data: [], reachable_areas: [] });
      }
      return json({});
    }),
  );
  return sent;
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <PersonalChannelsCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  pretendReaderZone(UTC_ZONE);
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

describe("PersonalChannelsCard", () => {
  it("is off at first, shows the AI-assistant guide, and turns on", async () => {
    const sent = backend({ enabled: false, replies_waiting: 0 });
    const user = userEvent.setup();
    renderCard();
    await screen.findByText("Through your AI assistant");
    expect(screen.getByText("Not connected")).toBeInTheDocument();
    expect(screen.queryByText("Replies")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Turn on" }));
    expect(sent).toEqual([{ enabled: true }]);
    expect(
      await screen.findByRole("button", { name: "Turn off" }),
    ).toBeInTheDocument();
  });

  it("shows how many replies wait, and when the client was last seen", async () => {
    backend({
      enabled: true,
      replies_waiting: 3,
      oldest_reply_waiting_at: "2026-11-14T16:05:00Z",
      last_pull_at: "2026-11-16T09:00:00Z",
    });
    renderCard();
    expect(await screen.findByText("3 waiting")).toBeInTheDocument();
    expect(screen.getByText(/Client last seen/)).toBeInTheDocument();
  });
});
