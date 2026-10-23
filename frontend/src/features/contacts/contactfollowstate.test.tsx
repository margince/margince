/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ContactFollowState } from "./contactfollowstate";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const contactId = "0199f5c0-0000-7000-8000-000000000e01";

// The server's standing choice for this reader: stopped until a DELETE.
function stubServer() {
  let stopped = true;
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const method = input instanceof Request ? input.method : "GET";
      calls.push(method);
      if (method === "DELETE") {
        stopped = false;
        return new Response(null, { status: 204 });
      }
      return new Response(
        JSON.stringify({ state: stopped ? "stopped" : "following" }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }),
  );
  return calls;
}

function mount() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ContactFollowState contactId={contactId} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a contact the reader stopped following", () => {
  it("still says so after a reload, and Resume brings it back", async () => {
    const calls = stubServer();
    const first = mount();
    expect(await screen.findByText(en["contact.follow.stopped"])).toBeTruthy();
    first.unmount();

    mount();
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: en["contact.follow.resume"] }),
    );
    await waitFor(() => {
      expect(calls).toContain("DELETE");
      expect(screen.queryByText(en["contact.follow.stopped"])).toBeNull();
    });
  });

  it("shows nothing for a contact the reader follows", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ state: "following" }), {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    mount();
    await waitFor(() => {
      expect(globalThis.fetch).toHaveBeenCalled();
    });
    expect(
      screen.queryByRole("button", { name: en["contact.follow.resume"] }),
    ).toBeNull();
  });
});
