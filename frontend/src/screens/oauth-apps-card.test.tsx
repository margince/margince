/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { OAuthAppsCard } from "./oauth-apps-card";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type ConnectorApp = components["schemas"]["ConnectorApp"];

const GOOGLE_ID = "804215663017-brandtcrm.apps.googleusercontent.com";
const ENTRA_ID = "6d1e9b52-4c7a-4e0f-9a31-2b8f5c0d7e14";
const DIRECTORY = "9b0c3a71-58e2-4d16-b7a4-0f3e6c21d985";

function app(
  provider: ConnectorApp["provider"],
  source: ConnectorApp["source"],
  tenant?: string,
): ConnectorApp {
  return {
    provider,
    configured: source !== "none",
    client_id:
      source === "none" ? "" : provider === "google" ? GOOGLE_ID : ENTRA_ID,
    ...(tenant === undefined ? {} : { tenant }),
    source,
    redirect_uris: [
      {
        purpose: "mailbox_connect",
        url: `https://api.acme.test/v1/connectors/${provider}/callback`,
      },
    ],
  };
}

function mount(
  google: ConnectorApp,
  microsoft: ConnectorApp,
  canManage = true,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url).pathname;
      if (url.endsWith("/me")) {
        return jsonResponse(
          meFixture({
            allow: {
              capture_settings: canManage ? ["read", "update"] : ["read"],
            },
          }),
        );
      }
      return jsonResponse(url.endsWith("/google") ? google : microsoft);
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  render(<OAuthAppsCard />, { wrapper: Wrap });
}

// The row a vendor's name heads, so a test reads one vendor's facts only.
async function row(name: string): Promise<HTMLElement> {
  const heading = await screen.findByRole("heading", { name });
  const found = heading.closest<HTMLElement>(".provider-row");
  if (!found) {
    throw new Error(`no provider row holds the ${name} heading`);
  }
  return found;
}

describe("the Google and Microsoft apps card", () => {
  // The three sources are three answers: `configured` alone cannot tell a
  // stored app from one this installation's configuration supplies.
  it("names where each app comes from", async () => {
    mount(app("google", "stored"), app("microsoft", "environment"));
    const google = await row("Google app");
    expect(within(google).getByText("Set up")).toBeTruthy();
    expect(within(google).getByText(`Client ${GOOGLE_ID}`)).toBeTruthy();
    const microsoft = await row("Microsoft app");
    expect(
      within(microsoft).getByText("Set by this installation"),
    ).toBeTruthy();
    expect(
      within(microsoft).getByText(
        `Client ${ENTRA_ID}, from this installation’s configuration`,
      ),
    ).toBeTruthy();
  });

  it("says an app is not set up, and who it is for", async () => {
    mount(app("google", "none"), app("microsoft", "stored"));
    const google = await row("Google app");
    expect(within(google).getByText("Not set up")).toBeTruthy();
    expect(
      within(google).getByText("For companies on Google Workspace or Gmail."),
    ).toBeTruthy();
    expect(
      within(google).getByRole("button", { name: "Set up Google app" }),
    ).toBeTruthy();
  });

  // Nobody can connect mail until one vendor has an app, and that is the
  // finding the card leads with.
  it("reads Needs setup, and warns, only when neither vendor has an app", async () => {
    mount(app("google", "none"), app("microsoft", "none"));
    expect(await screen.findByText("Needs setup")).toBeTruthy();
    expect(
      screen.getByText("Nobody can connect Gmail, Outlook or a calendar yet"),
    ).toBeTruthy();
    expect(screen.queryByText("Ready")).toBeNull();
  });

  it("reads Ready as soon as one vendor has an app", async () => {
    mount(app("google", "none"), app("microsoft", "environment"));
    expect(await screen.findByText("Ready")).toBeTruthy();
    expect(screen.queryByText("Needs setup")).toBeNull();
    expect(
      screen.queryByText("Nobody can connect Gmail, Outlook or a calendar yet"),
    ).toBeNull();
  });

  // The pin decides who may connect, wherever the app came from, so it is
  // shown for a configuration-supplied app too.
  it("shows the directory a Microsoft app is limited to", async () => {
    mount(app("google", "none"), app("microsoft", "environment", DIRECTORY));
    const microsoft = await row("Microsoft app");
    expect(
      within(microsoft).getByText(
        `Limited to your company directory ${DIRECTORY}`,
      ),
    ).toBeTruthy();
  });

  it("offers to replace a configuration-supplied app with the company's own", async () => {
    mount(app("google", "none"), app("microsoft", "environment"));
    const microsoft = await row("Microsoft app");
    expect(
      within(microsoft).getByRole("button", { name: "Use your own app" }),
    ).toBeTruthy();
  });

  // Only a stored app can be replaced or removed, so its verbs live in a menu.
  it("puts a stored app's verbs in its menu, with Remove last", async () => {
    const user = userEvent.setup();
    mount(app("google", "stored"), app("microsoft", "none"));
    await user.click(
      await screen.findByRole("button", {
        name: "More actions for the Google app",
      }),
    );
    expect(
      await screen.findByRole("button", { name: "Show redirect addresses" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Replace client ID and secret" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Remove app" }));
    expect(
      await screen.findByRole("dialog", { name: "Remove the Google app?" }),
    ).toBeTruthy();
  });

  it("opens the setup drawer from Set up", async () => {
    const user = userEvent.setup();
    mount(app("google", "none"), app("microsoft", "none"));
    await user.click(
      await screen.findByRole("button", { name: "Set up Google app" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Set up the Google app" }),
    ).toBeTruthy();
  });

  // A reader who may not write the apps still sees the verb, refused with the
  // sentence that says why, rather than a page with the verbs missing.
  it("refuses Set up with its reason to a reader who cannot change apps", async () => {
    mount(app("google", "none"), app("microsoft", "none"), false);
    const setUp = await screen.findByRole("button", {
      name: "Set up Google app",
    });
    expect(setUp).toHaveProperty("disabled", true);
    expect(
      screen.getByText("Only an admin can set up company apps"),
    ).toBeTruthy();
  });
});
