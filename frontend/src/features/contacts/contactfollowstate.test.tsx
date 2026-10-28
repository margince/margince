// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ContactFollowState } from "./contactfollowstate";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const contactId = "0199f5c0-0000-7000-8000-000000000e01";

type FollowState = { state: string; dismissed_until?: string };

// The server's standing choice for this reader, until a DELETE resumes it.
// `refuseResume` answers the DELETE with a problem body, as a refusal does.
function stubServer(initial: FollowState, refuseResume = false) {
  let current = initial;
  const calls: { method: string; url: string }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const method = input instanceof Request ? input.method : "GET";
      const url = String(input instanceof Request ? input.url : input);
      calls.push({ method, url });
      if (method === "DELETE") {
        if (refuseResume) {
          return new Response(
            JSON.stringify({ title: "Forbidden", status: 403 }),
            { status: 403, headers: { "content-type": "application/json" } },
          );
        }
        current = { state: "following" };
        return new Response(null, { status: 204 });
      }
      return Response.json(current);
    }),
  );
  return calls;
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactFollowState contactId={contactId} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

describe("the reader's own choice about a quiet contact", () => {
  it("says the reader stopped following, and Resume deletes the stop", async () => {
    const calls = stubServer({ state: "stopped" });
    const user = userEvent.setup();
    mount();

    expect(await screen.findByText(en["contact.follow.stopped"])).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: en["contact.follow.resume"] }),
    );

    await waitFor(() => {
      expect(screen.queryByText(en["contact.follow.stopped"])).toBeNull();
    });
    expect(
      calls.filter((call) => call.method === "DELETE").map((call) => call.url),
    ).toEqual([
      expect.stringContaining(`/contacts/${contactId}/nudge-dismissal`),
    ]);
  });

  it("names the day a set-aside contact comes back, on the reader's clock", async () => {
    const until = "2026-12-30T23:30:00Z";
    stubServer({ state: "set_aside", dismissed_until: until });
    mount();

    expect(
      await screen.findByText(
        en["contact.follow.setAside"].replace(
          "{date}",
          formatDate(until, "en", viewerZone()),
        ),
      ),
    ).toBeTruthy();
  });

  it("keeps the notice and says so when the resume is refused", async () => {
    stubServer({ state: "stopped" }, true);
    const user = userEvent.setup();
    mount();

    await user.click(
      await screen.findByRole("button", { name: en["contact.follow.resume"] }),
    );

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByText(en["contact.follow.stopped"])).toBeTruthy();
  });

  it("draws nothing for a contact the reader follows", async () => {
    stubServer({ state: "following" });
    const client = mount();

    // Settled, so the absence below is the answer and not a pending read.
    await waitFor(() => {
      expect(client.isFetching()).toBe(0);
      expect(client.getQueryCache().getAll()[0]?.state.status).toBe("success");
    });
    expect(
      screen.queryByRole("button", { name: en["contact.follow.resume"] }),
    ).toBeNull();
  });
});
