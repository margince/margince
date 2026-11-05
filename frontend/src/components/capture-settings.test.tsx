/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider, translate } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { CaptureSettingsCard } from "./capture-settings";

// Every role reads the enrichment card and only admin or ops may change it, so
// a refusal disables the controls and never hides them.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The toggle is capture_settings:update server-side. Naming the grant rather
// than a role keeps the fixture honest about what the screen actually asks.
const CAPTURE_EDITOR: GrantSpec = { capture_settings: ["update"] };

const SETTINGS = {
  auto_enrich: true,
  mail_sharing: true,
  shared_posture_allowed: false,
  signature_enrich: true,
  auto_enrich_daily_cap: 500,
  site_read: { max_pages: 60, max_mib: 32, wall_seconds: 240 },
  mail_sync_interval_seconds: 120,
  confidentiality_pass_cap: 40,
};

// backendFor answers /me with the given grants and /capture/settings with the
// given auto_enrich, capturing any PATCH body so the wire shape is assertable.
function backendFor(allow: GrantSpec, autoEnrich = true) {
  let autoState = autoEnrich;
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
          autoState = (capturedPatch as { auto_enrich: boolean }).auto_enrich;
        }
        return jsonResponse({ ...SETTINGS, auto_enrich: autoState });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, getCapturedPatch: () => capturedPatch };
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("CaptureSettingsCard", () => {
  it("shows the current posture and an enabled toggle for admin", async () => {
    vi.stubGlobal("fetch", backendFor(CAPTURE_EDITOR, true).fetchMock);
    render(<CaptureSettingsCard />);

    // A setting that writes when you flip it is a switch, not a checkbox, so
    // its state is what it announces rather than a DOM property.
    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("capture-auto-enrich-toggle"),
    );
    expect(toggle.getAttribute("role")).toBe("switch");
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(toggle.disabled).toBe(false);
  });

  it("disables the toggle for a non-admin role", async () => {
    vi.stubGlobal("fetch", backendFor({}, true).fetchMock);
    render(<CaptureSettingsCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("capture-auto-enrich-toggle"),
    );
    expect(toggle.disabled).toBe(true);
    // Asserted through THIS toggle's own describedBy rather than by finding the
    // sentence on the page: every switch on the card carries the same refusal,
    // so a page-wide text match stopped telling them apart the moment there
    // were two — and what matters is that the reason reaches a reader focused
    // on the control they cannot use.
    expect(toggle).toHaveAccessibleDescription(
      /Only an administrator or operations user/,
    );
  });

  it("PATCHes the new value when admin toggles it off", async () => {
    const backend = backendFor(CAPTURE_EDITOR, true);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<CaptureSettingsCard />);

    const toggle = await waitFor(() =>
      screen.getByTestId<HTMLButtonElement>("capture-auto-enrich-toggle"),
    );
    await userEvent.click(toggle);

    await waitFor(() =>
      expect(backend.getCapturedPatch()).toEqual({ auto_enrich: false }),
    );
  });

  it("refuses a limit the environment sets, says where to change it, and leaves the rest editable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: CAPTURE_EDITOR }));
        }
        return jsonResponse({
          ...SETTINGS,
          locks: [
            {
              key: "capture.auto_enrich_daily_cap",
              origin: {
                source: "env",
                name: "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
              },
            },
          ],
        });
      }),
    );
    render(<CaptureSettingsCard />);

    const cap =
      await screen.findByTestId<HTMLInputElement>("capture-daily-cap");
    expect(cap.disabled).toBe(true);
    const reason = screen.getByText(
      "Set by MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP (environment). Change it there.",
    );
    expect(cap.getAttribute("aria-describedby")?.split(" ")).toContain(
      reason.id,
    );
    expect(
      screen.getByTestId<HTMLInputElement>("capture-read-max-pages").disabled,
    ).toBe(false);
    expect(
      screen.getByTestId<HTMLButtonElement>("capture-auto-enrich-toggle")
        .disabled,
    ).toBe(false);
  });
});

// A proxy that never reached the application answers with its own page, not
// with RFC 7807 — so the settings read fails carrying nothing a reader was
// meant to see. What fills that hole has to be catalog copy, in the reader's
// own language.
describe("a settings read refused without a problem body", () => {
  const GATEWAY_PAGE = "<html><body>502 Bad Gateway</body></html>";

  function stubGateway() {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: CAPTURE_EDITOR }));
        }
        return new Response(GATEWAY_PAGE, {
          status: 502,
          headers: { "Content-Type": "text/html" },
        });
      }),
    );
  }

  it("reads the shared failure line, never a placeholder written for developers", async () => {
    stubGateway();
    render(<CaptureSettingsCard />);

    expect(
      await screen.findByText(translate("en", "common.errorNoCause")),
    ).toBeTruthy();
    // The placeholder a ProblemError carries into a stack trace is not a
    // sentence for a reader, and the gateway's own page is not one either.
    expect(screen.queryByText("request failed")).toBeNull();
    expect(screen.queryByText(/Bad Gateway/)).toBeNull();
  });

  it("says it in the reader's language", async () => {
    stubGateway();
    render(<CaptureSettingsCard />, "de");

    expect(
      await screen.findByText(translate("de", "common.errorNoCause")),
    ).toBeTruthy();
  });
});

// The full capture-settings record, saving what each limit row writes.
function readingBackend(allow: GrantSpec) {
  let state = SETTINGS;
  const patches: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/capture/settings")) {
        if (req.method === "PATCH") {
          const patch = (await req.json()) as Record<string, number>;
          patches.push(patch);
          state = {
            ...state,
            auto_enrich_daily_cap:
              patch.auto_enrich_daily_cap ?? state.auto_enrich_daily_cap,
            site_read: {
              max_pages: patch.site_read_max_pages ?? state.site_read.max_pages,
              max_mib: patch.site_read_max_mib ?? state.site_read.max_mib,
              wall_seconds:
                patch.site_read_wall_seconds ?? state.site_read.wall_seconds,
            },
            mail_sync_interval_seconds:
              patch.mail_sync_interval_seconds ??
              state.mail_sync_interval_seconds,
          };
        }
        return jsonResponse(state);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, patches };
}

describe("the website-read limits on the enrichment card", () => {
  it("saves a new daily cap on Enter", async () => {
    const backend = readingBackend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<CaptureSettingsCard />);

    const cap =
      await screen.findByTestId<HTMLInputElement>("capture-daily-cap");
    expect(cap.value).toBe("500");
    await user.clear(cap);
    await user.type(cap, "5000{Enter}");

    await waitFor(() =>
      expect(backend.patches).toEqual([{ auto_enrich_daily_cap: 5000 }]),
    );
  });

  it("refuses a cap past the ceiling without sending it", async () => {
    const backend = readingBackend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<CaptureSettingsCard />);

    const cap =
      await screen.findByTestId<HTMLInputElement>("capture-daily-cap");
    await user.clear(cap);
    await user.type(cap, "20001{Enter}");

    expect(
      await screen.findByText(
        translate("en", "captureReading.dailyCap.refusal"),
      ),
    ).toBeTruthy();
    expect(backend.patches).toEqual([]);
  });

  it("shows the limits read-only to a seat without the update grant", async () => {
    vi.stubGlobal("fetch", readingBackend({}).fetchMock);
    render(<CaptureSettingsCard />);

    const pages = await screen.findByTestId<HTMLInputElement>(
      "capture-read-max-pages",
    );
    expect(pages.value).toBe("60");
    expect(pages.disabled).toBe(true);
    expect(
      within(screen.getByTestId("capture-website-reads")).getAllByText(
        translate("en", "captureSettings.adminOnly"),
      ),
    ).toHaveLength(4);
  });

  // Each row writes its own wire field and refuses on its own bounds: a row
  // wired to its neighbour's property would save into the wrong setting.
  it.each([
    ["capture-read-max-pages", "site_read_max_pages", "120", "201", "maxPages"],
    ["capture-read-max-mib", "site_read_max_mib", "64", "129", "maxMiB"],
    ["capture-read-wall", "site_read_wall_seconds", "300", "29", "wall"],
  ] as const)(
    "%s saves %s and refuses past its bounds",
    async (testId, property, ok, bad, copy) => {
      const backend = readingBackend(CAPTURE_EDITOR);
      vi.stubGlobal("fetch", backend.fetchMock);
      const user = userEvent.setup();
      render(<CaptureSettingsCard />);

      const box = await screen.findByTestId<HTMLInputElement>(testId);
      await user.clear(box);
      await user.type(box, `${bad}{Enter}`);
      expect(
        await screen.findByText(
          translate("en", `captureReading.${copy}.refusal`),
        ),
      ).toBeTruthy();
      expect(backend.patches).toEqual([]);

      await user.clear(box);
      await user.type(box, `${ok}{Enter}`);
      await waitFor(() =>
        expect(backend.patches).toEqual([{ [property]: Number(ok) }]),
      );
    },
  );
});

describe("a refused save on the enrichment card", () => {
  const UNAVAILABLE = {
    type: "about:blank",
    title: "Service Unavailable",
    status: 503,
    code: "service_unavailable",
    detail: "The settings store is not reachable.",
  };

  function refusingBackend() {
    return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: CAPTURE_EDITOR }));
      }
      return req.method === "PATCH"
        ? jsonResponse(UNAVAILABLE, 503)
        : jsonResponse(SETTINGS);
    });
  }

  const notice = expect.stringContaining(
    translate("en", "captureSettings.updateFailed"),
  );

  it("names the notice on the limit it refused, after the row's description", async () => {
    vi.stubGlobal("fetch", refusingBackend());
    const user = userEvent.setup();
    render(<CaptureSettingsCard />);
    const cap =
      await screen.findByTestId<HTMLInputElement>("capture-daily-cap");
    const pages = screen.getByTestId("capture-read-max-pages");
    const before = cap.getAttribute("aria-describedby");
    await user.clear(cap);
    await user.type(cap, "5000{Enter}");

    const alert = await screen.findByRole("alert");
    expect(cap.getAttribute("aria-describedby")).toBe(`${before} ${alert.id}`);
    expect(cap).toHaveAccessibleDescription(notice);
    expect(pages.getAttribute("aria-describedby")).not.toContain(alert.id);
    expect(
      screen.getByTestId("capture-auto-enrich-toggle"),
    ).not.toHaveAccessibleDescription(notice);
  });

  it("names each switch's row description before any write", async () => {
    vi.stubGlobal("fetch", refusingBackend());
    render(<CaptureSettingsCard />);

    expect(
      await screen.findByTestId("capture-auto-enrich-toggle"),
    ).toHaveAccessibleDescription(
      translate("en", "captureSettings.autoEnrich.help"),
    );
    expect(
      screen.getByTestId("capture-signature-enrich-toggle"),
    ).toHaveAccessibleDescription(
      translate("en", "captureSettings.signatureEnrich.help"),
    );
  });

  it("names the notice on the switch it refused, after its row, and no other", async () => {
    vi.stubGlobal("fetch", refusingBackend());
    const user = userEvent.setup();
    render(<CaptureSettingsCard />);
    const toggle = await screen.findByTestId("capture-auto-enrich-toggle");
    await user.click(toggle);

    await screen.findByRole("alert");
    expect(toggle).toHaveAccessibleDescription(
      expect.stringMatching(
        new RegExp(
          `^${translate("en", "captureSettings.autoEnrich.help")}.*${translate("en", "captureSettings.updateFailed")}`,
        ),
      ),
    );
    expect(
      screen.getByTestId("capture-signature-enrich-toggle"),
    ).not.toHaveAccessibleDescription(notice);
    expect(
      screen.getByTestId("capture-daily-cap"),
    ).not.toHaveAccessibleDescription(notice);
  });
});
