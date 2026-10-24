/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import type { GrantSpec } from "../../testing/mefixture";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
import { ConnectorsCard } from "./connectors";

// The Mail and calendar card before anything is connected. It shows the steps
// above a tile per vendor whose Connect opens the dialog on that vendor. A
// vendor this installation has not wired is marked on its own tile with who
// turns it on. Below sit the row for another mail host and the company's
// sharing rule.

type Availability = components["schemas"]["CaptureProviderAvailability"];

function stubEmptyRoster(
  providers: Availability[] = [],
  allow: GrantSpec = {},
) {
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ data: [], providers }),
      "GET /capture/settings": () =>
        jsonResponse({
          auto_enrich: true,
          mail_sharing: true,
          shared_posture_allowed: false,
          signature_enrich: true,
        }),
    },
    allow,
  );
}

function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function mailPanel() {
  const heading = screen.getByRole("heading", { name: "Mail and calendar" });
  const panel = heading.closest("section");
  if (!panel) throw new Error("the mail heading is not inside its panel");
  return panel;
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the empty Mail and calendar card", () => {
  it("leads with a tile per vendor and says nothing is connected", async () => {
    stubEmptyRoster();
    render(<ConnectorsCard />);

    await screen.findByTestId("connect-tile-google");
    const panel = mailPanel();
    expect(within(panel).getByText("Not connected")).toBeInTheDocument();
    // Adding is what the tiles are for, so the header verb waits for an account.
    expect(
      within(panel).queryByRole("button", { name: "Add account" }),
    ).toBeNull();
    for (const vendor of ["Google", "Microsoft 365"]) {
      expect(
        within(panel).getByRole("button", { name: `Connect ${vendor}` }),
      ).toBeEnabled();
    }
    // The steps are read once, before the tiles they describe.
    const steps = within(panel).getByRole("list", {
      name: "How connecting works",
    });
    expect(
      steps.compareDocumentPosition(screen.getByTestId("connect-tile-google")),
    ).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });

  it("opens the dialog on what the pressed vendor brings in", async () => {
    stubEmptyRoster();
    render(<ConnectorsCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Connect Microsoft 365" }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Connect Microsoft 365",
    });
    // The account step is skipped: the tile already chose it.
    expect(within(dialog).queryByRole("radio", { name: /^Google/ })).toBeNull();
    expect(
      within(dialog).getByRole("checkbox", { name: /^Mail/ }),
    ).toBeChecked();
    expect(
      within(dialog).getByRole("checkbox", { name: /^Calendar/ }),
    ).toBeChecked();
  });

  it("marks a vendor this installation has not wired and names who turns it on", async () => {
    stubEmptyRoster([
      { provider: "gmail", reason: "ready" },
      { provider: "graph", reason: "app_missing" },
    ]);
    render(<ConnectorsCard />);

    const microsoft = await screen.findByTestId("connect-tile-microsoft");
    expect(within(microsoft).getByText("Not set up")).toBeInTheDocument();
    expect(
      within(microsoft).getByText(
        "An administrator turns Microsoft 365 on in Sign-in and apps.",
      ),
    ).toBeInTheDocument();
    // Nothing on the tile can start a connect that would only be refused.
    expect(within(microsoft).queryByRole("button")).toBeNull();
    expect(within(microsoft).queryByRole("link")).toBeNull();
    expect(
      within(screen.getByTestId("connect-tile-google")).getByRole("button", {
        name: "Connect Google",
      }),
    ).toBeEnabled();
  });

  it("hands whoever can register the company app the way to Sign-in and apps", async () => {
    stubEmptyRoster(
      [
        { provider: "gmail", reason: "ready" },
        { provider: "graph", reason: "app_missing" },
      ],
      { capture_settings: ["update"] },
    );
    render(<ConnectorsCard />);

    const microsoft = await screen.findByTestId("connect-tile-microsoft");
    const link = await within(microsoft).findByRole("link", {
      name: "Open Sign-in and apps",
    });
    expect(link).toHaveAttribute("href", "#/settings/authentication");
    expect(
      within(microsoft).getByText(
        "Set up the company Microsoft 365 app to turn it on.",
      ),
    ).toBeInTheDocument();
  });

  it("takes another mail host through the dialog to the IMAP form", async () => {
    stubEmptyRoster();
    render(<ConnectorsCard />);

    await userEvent.click(
      await screen.findByRole("button", {
        name: "Connect with IMAP",
      }),
    );
    const dialog = screen.getByRole("dialog", {
      name: "Connect another mail host",
    });
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Continue" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Connect IMAP mailbox" }),
    ).toBeInTheDocument();
  });

  it("states the company's sharing rule under the card", async () => {
    stubEmptyRoster();
    render(<ConnectorsCard />);

    await screen.findByTestId("connect-tile-google");
    const panel = mailPanel();
    expect(
      await within(panel).findByText(
        /shares newly captured mail with colleagues/,
      ),
    ).toBeInTheDocument();
    expect(within(panel).getByText("Whole company")).toBeInTheDocument();
  });
});
