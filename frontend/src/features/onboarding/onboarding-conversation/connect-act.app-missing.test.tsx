/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { stubClipboard } from "../../../design-system/clipboard-testing";
import { LocaleProvider } from "../../../i18n";
import type { GrantSpec } from "../../../testing/mefixture";
import type { RouteMap } from "../../../testing/story-utils";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { ConnectAct } from "./connect-act";
import type { ProviderAvailability } from "./connect-scene";
import type { ConversationState } from "./conversation-machine";
import { initialConversationState } from "./conversation-machine";

// An account whose company app is missing or will not open. The onboarding
// gate bounces any link to Settings back to this step, so the fix lives here.
// An administrator registers the app in place; anyone else hands the request
// on or picks another mail provider.

function renderAct() {
  const state: ConversationState = {
    ...initialConversationState,
    act: "connect",
    phase: "cn.consent",
  };
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConnectAct
          state={state}
          dispatch={vi.fn()}
          persist={vi.fn(async () => true)}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function roster(providers: ProviderAvailability[]) {
  return () => jsonResponse({ data: [], providers });
}

const GOOGLE_MISSING: ProviderAvailability[] = [
  { provider: "gmail", reason: "app_missing" },
  { provider: "graph", reason: "ready" },
  { provider: "imap", reason: "ready" },
];

function stub(
  providers: ProviderAvailability[],
  allow: GrantSpec = {},
  routes: RouteMap = {},
) {
  stubWithSession({ "GET /connectors": roster(providers), ...routes }, allow);
}

async function pick(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  const radio = await screen.findByRole("radio", { name });
  await waitFor(() => expect(radio).not.toBeDisabled());
  await user.click(radio);
}

let assign: ReturnType<typeof vi.fn>;
beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
  assign = vi.fn();
  vi.stubGlobal("location", { ...globalThis.location, assign });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

describe("an account waiting on an administrator", () => {
  // Picking the account is how the reader learns who fixes it, so it stays
  // pickable; the accounts open now say so before anything is picked.
  it("badges the waiting account and the ones open now", async () => {
    stub(GOOGLE_MISSING);
    renderAct();

    const google = await screen.findByRole("radio", {
      name: /Google.*Waiting for an administrator/,
    });
    await waitFor(() => expect(google).not.toBeDisabled());
    expect(
      screen.getByRole("radio", { name: /Other mail provider.*Available now/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("radio", { name: /Microsoft.*Available now/ }),
    ).toBeInTheDocument();
  });

  it("draws no standing at all while every account is ready", async () => {
    stub([
      { provider: "gmail", reason: "ready" },
      { provider: "graph", reason: "ready" },
      { provider: "imap", reason: "ready" },
    ]);
    renderAct();

    await screen.findByRole("radio", { name: /Google/ });
    expect(screen.queryByText("Available now")).toBeNull();
    expect(screen.queryByText("Waiting for an administrator")).toBeNull();
  });
});

describe("a reader who cannot register the app", () => {
  it("copies a request for the administrator instead of a dead link", async () => {
    stub(GOOGLE_MISSING);
    renderAct();
    const user = userEvent.setup();
    const clipboard = stubClipboard("accepts");
    await pick(user, /Google/);

    expect(
      screen.getByRole("heading", {
        name: "Google is not set up for your company yet",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Before anyone at Test Workspace/)).toBeVisible();
    // Nothing on the step may press into a consent that would be refused.
    expect(
      screen.queryByRole("button", { name: "Connect mail and calendar" }),
    ).toBeNull();
    // A link to Settings is a dead end: the onboarding gate sends it back.
    expect(screen.queryByRole("link")).toBeNull();
    const request =
      "Could you register the company’s Google app in Margince, in Settings under Sign-in and apps? Then I can connect my mail and calendar.";
    // The reader sees what they are about to send before they copy it.
    expect(screen.getByText(request)).toBeVisible();

    await user.click(
      screen.getByRole("button", {
        name: "Copy request for your administrator",
      }),
    );
    await waitFor(() => expect(clipboard.written).toEqual([request]));
    expect(
      screen.getByRole("button", { name: "Request copied" }),
    ).toBeInTheDocument();
  });

  // The way round the wait is one press, not a hunt back up the page.
  it("moves to another mail provider", async () => {
    stub(GOOGLE_MISSING);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    await user.click(
      screen.getByRole("button", { name: "Use another mail provider" }),
    );

    expect(
      screen.getByRole("radio", { name: /Other mail provider/ }),
    ).toBeChecked();
    expect(
      await screen.findByRole("heading", { name: "Connect mail host" }),
    ).toBeInTheDocument();
  });

  it("offers no other provider when this installation serves none", async () => {
    stub([
      { provider: "gmail", reason: "app_missing" },
      { provider: "graph", reason: "app_missing" },
      { provider: "imap", reason: "unsupported" },
    ]);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    expect(
      screen.getByRole("button", {
        name: "Copy request for your administrator",
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Use another mail provider" }),
    ).toBeNull();
  });

  // A stored app whose secret will not open is a different fix from no app at
  // all. Asking for a new app sends the administrator to register a second one.
  it("asks for a new client secret when the stored app will not open", async () => {
    stub([
      { provider: "gmail", reason: "ready" },
      { provider: "graph", reason: "app_unusable" },
      { provider: "imap", reason: "ready" },
    ]);
    renderAct();
    const user = userEvent.setup();
    const clipboard = stubClipboard("accepts");
    await pick(user, /Microsoft/);

    expect(
      screen.getByRole("heading", {
        name: "Microsoft cannot be connected right now",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText(/is not set up/)).toBeNull();
    await user.click(
      screen.getByRole("button", {
        name: "Copy request for your administrator",
      }),
    );
    await waitFor(() =>
      expect(clipboard.written[0]).toMatch(
        /^Could you replace the client secret of the company’s Microsoft app/,
      ),
    );
  });

  // The panel's request is the page's one primary; the rail offers only the
  // skip, and its caption says the checklist waits for the vendor.
  it("keeps one primary and names the vendor on the rail", async () => {
    stub(GOOGLE_MISSING);
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    const primaries = document.querySelectorAll(".btn-primary");
    expect(primaries).toHaveLength(1);
    expect(primaries[0]).toHaveTextContent(
      "Copy request for your administrator",
    );
    expect(
      screen.getByText(
        "Your setup checklist reminds you to connect once Google is ready.",
      ),
    ).toBeInTheDocument();
  });
});

describe("an administrator", () => {
  it("is told the account needs setup, never to wait for an administrator", async () => {
    stub(GOOGLE_MISSING, { capture_settings: ["read", "update"] });
    renderAct();

    await screen.findByRole("radio", { name: /Google.*Needs setup/ });
    expect(screen.queryByText("Waiting for an administrator")).toBeNull();
  });

  // Registering the app in place survives the onboarding gate, and the
  // roster it refreshes turns the account connectable where it stands.
  it("registers the app in place and the account opens", async () => {
    let registered = false;
    const puts: unknown[] = [];
    stub(
      GOOGLE_MISSING,
      { capture_settings: ["read", "update"] },
      {
        "GET /connectors": () =>
          jsonResponse({
            data: [],
            providers: registered
              ? [
                  { provider: "gmail", reason: "ready" },
                  ...GOOGLE_MISSING.slice(1),
                ]
              : GOOGLE_MISSING,
          }),
        "GET /installation/oauth-apps/google": () =>
          jsonResponse({
            provider: "google",
            configured: registered,
            client_id: "",
            source: "none",
            redirect_uris: [
              {
                purpose: "mailbox_connect",
                url: "https://api.acme.test/v1/connectors/google/callback",
              },
            ],
          }),
        "PUT /installation/oauth-apps/google": (body: unknown) => {
          puts.push(body);
          registered = true;
          return new Response(null, { status: 204 });
        },
      },
    );
    renderAct();
    const user = userEvent.setup();
    await pick(user, /Google/);

    expect(
      screen.queryByRole("button", {
        name: "Copy request for your administrator",
      }),
    ).toBeNull();
    await user.click(
      screen.getByRole("button", { name: "Register the company’s Google app" }),
    );
    const drawer = await screen.findByRole("dialog");
    await user.type(
      await screen.findByLabelText("Client ID"),
      "111-abc.apps.googleusercontent.com",
    );
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-secret");
    await user.click(screen.getByRole("button", { name: "Save Google app" }));

    await waitFor(() => expect(drawer).not.toBeInTheDocument());
    expect(puts).toEqual([
      {
        client_id: "111-abc.apps.googleusercontent.com",
        client_secret: "GOCSPX-secret",
      },
    ]);
    // The same pick, now an ask: nothing navigated and nothing was re-picked.
    expect(
      await screen.findByRole("button", { name: "Connect mail and calendar" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Waiting for an administrator")).toBeNull();
    expect(assign).not.toHaveBeenCalled();
  });
});
