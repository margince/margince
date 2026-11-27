/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { SignedInDevicesPanel } from "./signeddevices";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CHROME_MAC =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36";
const SAFARI_IPHONE =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1";

const THIS_ID = "00000000-0000-4000-8000-000000000001";
const PHONE_ID = "00000000-0000-4000-8000-000000000002";
const LAPTOP_ID = "00000000-0000-4000-8000-000000000003";

function session(id: string, userAgent: string | null, current = false) {
  return {
    id,
    user_agent: userAgent,
    signed_in_at: "2026-11-01T08:00:00Z",
    last_active_at: "2026-11-02T09:30:00Z",
    current,
  };
}

function serve(sessions: ReturnType<typeof session>[]) {
  const ended: string[] = [];
  let live = sessions;
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({})),
    "GET /me/sessions": () => jsonResponse({ sessions: live }),
    [`DELETE /me/sessions/${PHONE_ID}`]: () => {
      ended.push(PHONE_ID);
      live = live.filter((entry) => entry.id !== PHONE_ID);
      return new Response(null, { status: 204 });
    },
    [`DELETE /me/sessions/${LAPTOP_ID}`]: () => {
      ended.push(LAPTOP_ID);
      live = live.filter((entry) => entry.id !== LAPTOP_ID);
      return new Response(null, { status: 204 });
    },
    [`DELETE /me/sessions/${THIS_ID}`]: () => {
      ended.push(THIS_ID);
      return new Response(null, { status: 204 });
    },
  });
  return ended;
}

function draw() {
  render(
    <StoryProviders>
      <SignedInDevicesPanel />
    </StoryProviders>,
  );
}

it("names each device, marks this one and offers no sign out for it", async () => {
  serve([session(THIS_ID, CHROME_MAC, true), session(PHONE_ID, SAFARI_IPHONE)]);
  draw();
  await screen.findByText("Chrome on macOS");
  expect(screen.getByText("This device")).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /Sign out Chrome on macOS/ }),
  ).toBeNull();
  expect(
    screen.getByRole("button", { name: "Sign out Safari on iOS" }),
  ).toBeInTheDocument();
  expect(screen.getAllByText(/Last active/)).toHaveLength(2);
});

it("says unknown device when the session sent no user agent", async () => {
  serve([session(THIS_ID, null, true)]);
  draw();
  expect(
    await screen.findByText(englishCopy["account.devicesUnknown"]),
  ).toBeInTheDocument();
});

it("ends one other session and drops its row", async () => {
  const user = userEvent.setup();
  const ended = serve([
    session(THIS_ID, CHROME_MAC, true),
    session(PHONE_ID, SAFARI_IPHONE),
  ]);
  draw();
  await user.click(
    await screen.findByRole("button", { name: "Sign out Safari on iOS" }),
  );
  await vi.waitFor(() => expect(ended).toEqual([PHONE_ID]));
  await vi.waitFor(() =>
    expect(screen.queryByText("Safari on iOS")).toBeNull(),
  );
  expect(screen.getByText("Chrome on macOS")).toBeInTheDocument();
});

it("signs out all others without ending this session", async () => {
  const user = userEvent.setup();
  const ended = serve([
    session(THIS_ID, CHROME_MAC, true),
    session(PHONE_ID, SAFARI_IPHONE),
    session(LAPTOP_ID, CHROME_MAC),
  ]);
  draw();
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["account.devicesSignOutOthers"],
    }),
  );
  await vi.waitFor(() => expect(ended).toEqual([PHONE_ID, LAPTOP_ID]));
  expect(ended).not.toContain(THIS_ID);
});

it("offers no sign out all others when this is the only session", async () => {
  serve([session(THIS_ID, CHROME_MAC, true)]);
  draw();
  await screen.findByText("Chrome on macOS");
  expect(
    screen.queryByRole("button", {
      name: englishCopy["account.devicesSignOutOthers"],
    }),
  ).toBeNull();
});

it("says so when a session could not be ended", async () => {
  const user = userEvent.setup();
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({})),
    "GET /me/sessions": () =>
      jsonResponse({
        sessions: [
          session(THIS_ID, CHROME_MAC, true),
          session(PHONE_ID, SAFARI_IPHONE),
        ],
      }),
    [`DELETE /me/sessions/${PHONE_ID}`]: () =>
      new Response(
        JSON.stringify({
          status: 503,
          code: "service_unavailable",
          detail: "The session could not be ended.",
        }),
        {
          status: 503,
          headers: { "content-type": "application/problem+json" },
        },
      ),
  });
  draw();
  await user.click(
    await screen.findByRole("button", { name: "Sign out Safari on iOS" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "The session could not be ended.",
  );
  expect(screen.getByText("Safari on iOS")).toBeInTheDocument();
});

it("tries every other session and says how many could not be signed out", async () => {
  const user = userEvent.setup();
  const tried: string[] = [];
  const refuse = (id: string) => () => {
    tried.push(id);
    return new Response(
      JSON.stringify({
        status: 503,
        code: "service_unavailable",
        detail: "Down.",
      }),
      { status: 503, headers: { "content-type": "application/problem+json" } },
    );
  };
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({})),
    "GET /me/sessions": () =>
      jsonResponse({
        sessions: [
          session(THIS_ID, CHROME_MAC, true),
          session(PHONE_ID, SAFARI_IPHONE),
          session(LAPTOP_ID, CHROME_MAC),
        ],
      }),
    [`DELETE /me/sessions/${PHONE_ID}`]: refuse(PHONE_ID),
    [`DELETE /me/sessions/${LAPTOP_ID}`]: () => {
      tried.push(LAPTOP_ID);
      return new Response(null, { status: 204 });
    },
  });
  draw();
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["account.devicesSignOutOthers"],
    }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "1 device was not signed out. Retry.",
  );
  const byId = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);
  expect([...tried].sort(byId)).toEqual([PHONE_ID, LAPTOP_ID].sort(byId));
});
