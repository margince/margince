/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { LicenseCard } from "./license";

// "Check now" on Settings → Seats: the api reads the deployment's license token
// again at once. The button is for whoever may change installation settings,
// and its answer replaces what the card shows.

const ABSENT = {
  state: "absent",
  seats_used: 3,
  over_limit: false,
  checked_at: "2026-08-14T09:00:00Z",
};
const REFUSED = { ...ABSENT, state: "rejected" };

const ADMIN: GrantSpec = {
  license: ["read"],
  installation_settings: ["read", "update"],
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// checkAnswer is what POST /installation/license/check answers; every call to
// it is recorded so a test can say whether the button was ever pressed.
function backend(allow: GrantSpec, checkAnswer: () => Response) {
  const checks: string[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url, "http://localhost");
    if (url.pathname.endsWith("/v1/me")) {
      return json(meFixture({ allow }));
    }
    if (url.pathname.endsWith("/installation/license/check")) {
      checks.push(req.method);
      return checkAnswer();
    }
    if (url.pathname.endsWith("/installation/license")) {
      return json(ABSENT);
    }
    throw new Error(`unexpected request: ${req.method} ${url.pathname}`);
  });
  vi.stubGlobal("fetch", fetch);
  return checks;
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <LicenseCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("LicenseCard Check now", () => {
  it("re-reads the license and shows the posture the server answers", async () => {
    const checks = backend(ADMIN, () => json(REFUSED));
    const user = userEvent.setup();
    render();

    await waitFor(() => screen.getByText(en["license.absent.title"]));
    await user.click(
      screen.getByRole("button", { name: en["license.check.button"] }),
    );

    expect(
      await waitFor(() => screen.getByText(en["license.check.done"])),
    ).toBeTruthy();
    expect(checks).toEqual(["POST"]);
    // The card shows the answer, not the posture it loaded with.
    expect(screen.getByText(en["license.refused.title"])).toBeTruthy();
    expect(screen.queryByText(en["license.absent.title"])).toBeNull();
  });

  it("says the token could not be read when the server answers 503", async () => {
    backend(ADMIN, () =>
      json(
        {
          code: "service_unavailable",
          detail: "server-side text",
          status: 503,
        },
        503,
      ),
    );
    const user = userEvent.setup();
    render();

    await waitFor(() => screen.getByText(en["license.absent.title"]));
    await user.click(
      screen.getByRole("button", { name: en["license.check.button"] }),
    );

    const line = await waitFor(() =>
      screen.getByText(en["license.check.failed"]),
    );
    expect(line.getAttribute("role")).toBe("status");
    expect(screen.queryByText("server-side text")).toBeNull();
    // The last result stands.
    expect(screen.getByText(en["license.absent.title"])).toBeTruthy();
  });

  it("offers no button to a reader who may not change installation settings", async () => {
    const checks = backend({ license: ["read"] }, () => json(REFUSED));
    render();

    await waitFor(() => screen.getByText(en["license.absent.title"]));
    expect(
      screen.queryByRole("button", { name: en["license.check.button"] }),
    ).toBeNull();
    expect(checks).toEqual([]);
  });
});
