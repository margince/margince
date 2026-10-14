/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { MailSharingCard, MailSharingPlate } from "./mail-sharing";

// The workspace mail-sharing posture (Settings > Capture rules): every role
// reads it, only capture_settings:update holders change it, each switch writes
// on flip, and the cost of the posture stays stated beside it.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const SETTINGS_EDITOR: GrantSpec = { capture_settings: ["update"] };

// backendFor answers /me with the given grants and /capture/settings with the
// given posture, capturing any PATCH body so the wire shape is assertable.
function backendFor(
  allow: GrantSpec,
  mailSharing = true,
  sharedAllowed = false,
  refuse = false,
) {
  let sharingState = mailSharing;
  let sharedState = sharedAllowed;
  let capturedPatch: unknown = null;
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/capture/settings")) {
        if (req.method === "PATCH") {
          capturedPatch = await req.json();
          if (refuse) {
            return jsonResponse(
              { title: "Forbidden", detail: "the posture is locked" },
              403,
            );
          }
          // The card saves its two settings apart, so a PATCH carries one of
          // them and leaves the other where it was.
          const body = capturedPatch as {
            mail_sharing?: boolean;
            shared_posture_allowed?: boolean;
          };
          sharingState = body.mail_sharing ?? sharingState;
          sharedState = body.shared_posture_allowed ?? sharedState;
        }
        return jsonResponse({
          auto_enrich: true,
          mail_sharing: sharingState,
          shared_posture_allowed: sharedState,
          signature_enrich: true,
        });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, getCapturedPatch: () => capturedPatch };
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("MailSharingCard", () => {
  it("renders the stored ON posture with no warning and no Save", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_EDITOR, true).fetchMock);
    render(<MailSharingCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("mail-sharing-toggle"),
    );
    expect(toggle.getAttribute("role")).toBe("switch");
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(toggle.disabled).toBe(false);
    // No unsaved change and nothing to warn about: the stored posture is ON.
    expect(screen.queryByText(/the CRM is hard to use/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("writes on flip to off and states what the held posture costs", async () => {
    const backend = backendFor(SETTINGS_EDITOR, true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<MailSharingCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("mail-sharing-toggle"),
    );
    await userEvent.click(toggle);

    await waitFor(() =>
      expect(backend.getCapturedPatch()).toEqual({ mail_sharing: false }),
    );
    expect(await screen.findByText(/the CRM is hard to use/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("writes the shared-posture opt-in on its own, with the works-council warning", async () => {
    const backend = backendFor(SETTINGS_EDITOR, true, false);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<MailSharingCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("shared-posture-allowed-toggle"),
    );
    // Off by default, and the warning belongs to ON: it is the permissive
    // answer on this row, unlike the sharing switch above it.
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(screen.queryByText(/works-council/)).toBeNull();

    await user.click(toggle);
    // The PATCH carries ONLY this setting: the opt-in must not also rewrite
    // the workspace sharing posture the reader did not touch.
    await waitFor(() =>
      expect(backend.getCapturedPatch()).toEqual({
        shared_posture_allowed: true,
      }),
    );
    expect(await screen.findByText(/works-council/)).toBeTruthy();
  });

  it("says a refused flip once for the card and shows the stored posture again", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(SETTINGS_EDITOR, true, false, true).fetchMock,
    );
    render(<MailSharingCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("mail-sharing-toggle"),
    );
    await userEvent.click(toggle);

    expect(await screen.findByText("Setting not saved")).toBeTruthy();
    await waitFor(() =>
      expect(toggle.getAttribute("aria-checked")).toBe("true"),
    );
  });

  it("disables the switch for a role without capture_settings update", async () => {
    vi.stubGlobal("fetch", backendFor({}, true).fetchMock);
    render(<MailSharingCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("mail-sharing-toggle"),
    );
    expect(toggle.disabled).toBe(true);
    // Both switches on this card carry the same refusal now, so the count is
    // what the assertion can state: the sentence reaches the reader for the row
    // under test, and matching one of two identical strings by text alone would
    // not say which.
    expect(
      screen.getAllByText(/Only an administrator or operations user/),
    ).toHaveLength(2);
  });
});

// The rule as it reads under the reader's own Mail and calendar card: a
// statement about the whole company, and the way to change it only for a
// reader who may.
describe("MailSharingPlate", () => {
  it("says the company shares, scopes it to the whole company and links an editor to the switch", async () => {
    vi.stubGlobal("fetch", backendFor(SETTINGS_EDITOR, true).fetchMock);
    render(<MailSharingPlate />);

    expect(
      await screen.findByText(
        /^Test Workspace shares newly captured mail with colleagues/,
      ),
    ).toBeTruthy();
    expect(screen.getByText("Whole company")).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Change in Capture rules" })
        .getAttribute("href"),
    ).toBe("#/settings/capture");
  });

  it("states a held posture and offers no link to a reader who cannot change it", async () => {
    vi.stubGlobal("fetch", backendFor({}, false).fetchMock);
    render(<MailSharingPlate />);

    expect(
      await screen.findByText(/^Test Workspace holds newly captured mail/),
    ).toBeTruthy();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
