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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { stubClipboard } from "../design-system/clipboard-testing";
import { LocaleProvider } from "../i18n";
import { SettingsScreen, settingsAddress } from "./settings";

// Minting a passport, in its own file rather than as more of `settings.test.tsx`
// — that one is already well past the 1000-line ceiling the frontend guide sets,
// and a suite stops being navigable long before it stops running.
//
// The flow issues a credential and discloses it EXACTLY ONCE, and it had no
// test at all while it was a row of controls inside the card. What is held here
// is what the mint dialog owes a reader: a named group of choices, a refusal it
// cannot submit past, the token surfaced where it cannot be missed, and a
// dialog that never takes that token away by closing.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  Reflect.deleteProperty(URL, "createObjectURL");
  Reflect.deleteProperty(URL, "revokeObjectURL");
  globalThis.localStorage.clear();
  globalThis.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const render = (tab: string) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SettingsScreen route={settingsAddress(tab)} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

// The POST answers with a token exactly once, and the caller decides whether it
// succeeds or hangs, so a refusal and an in-flight attempt are both reachable
// without a second fixture.
const API_BASE = "https://crm.example.test/v1";

function mintBackend(
  opts: {
    refuse?: boolean;
    hang?: boolean;
    expired?: boolean;
    bundle?: "serves" | "refuses";
  } = {},
): ReturnType<typeof vi.fn> {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    const method = input instanceof Request ? input.method : "GET";
    if (url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "ada@acme.test" },
        roles: ["admin"],
        teams: [],
      });
    }
    if (url.includes("/passports") && method === "POST") {
      // Never settles, so the pending branch is a real state rather than a
      // window a test has to race.
      if (opts.hang) return new Promise<Response>(() => {});
      if (opts.expired) {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Unauthorized",
            status: 401,
            detail: "Your session has expired.",
          },
          401,
        );
      }
      if (opts.refuse) {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Forbidden",
            status: 403,
            detail: "Your seat cannot lend that scope.",
          },
          403,
        );
      }
      return jsonResponse({
        id: "pp-new",
        label: "Scout",
        scopes: ["read", "draft"],
        created_at: "2026-08-01T08:00:00Z",
        expires_at: null,
        revoked_at: null,
        token: "mgp_live_0f3a91c4",
      });
    }
    if (url.endsWith("/v1/agent-bundle")) {
      if (opts.bundle === "refuses") {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Service Unavailable",
            status: 503,
            detail: "Set MARGINCE_PUBLIC_BASE_URL to serve the skill.",
          },
          503,
        );
      }
      return new Response(new Blob(["PK"]), {
        status: 200,
        headers: {
          "Content-Type": "application/zip",
          "Content-Disposition": 'attachment; filename="margince-skill.zip"',
        },
      });
    }
    return jsonResponse({
      data: [],
      api_base_url: API_BASE,
      page: { next_cursor: null, has_more: false },
    });
  });
}

// One instance per test, never per interaction: a second instance silently
// forgets which keys and buttons the first left held.
//
// The CARD's verb — in its header band, beside the title, since minting is what
// the card is for rather than one of the credentials it lists — names the THING
// it creates ("New passport") while the dialog's submit names the act ("Mint
// passport"), so the two are never one name for two acts. That is what lets
// every assertion below name the button it means.
async function openMintDialog(user: ReturnType<typeof userEvent.setup>) {
  render("agents");
  await user.click(await screen.findByRole("button", { name: "New passport" }));
  return screen.findByRole("dialog");
}

describe("PassportCard — minting", () => {
  it("puts the form in a confirm-width dialog, with the scopes as a named group", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend());
    const dialog = await openMintDialog(user);
    // A secret reveal takes the confirm shape.
    expect(dialog).toHaveClass("modal-confirm");

    // The name field is a real label, not a span pointed at by
    // aria-labelledby: clicking the words has to focus the control.
    const name = within(dialog).getByLabelText("Agent name");
    await user.click(within(dialog).getByText("Agent name"));
    expect(name).toHaveFocus();

    // Five choices that belong together ARE a group, and the group has a name.
    const group = within(dialog).getByRole("group", {
      name: /agent permissions/i,
    });
    expect(within(group).getAllByRole("checkbox")).toHaveLength(5);
  });

  it("refuses a passport with no scope, and says why", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend());
    const dialog = await openMintDialog(user);
    const group = within(dialog).getByRole("group", {
      name: /agent permissions/i,
    });

    // read and draft are on by default; clearing both leaves a credential that
    // could do nothing.
    for (const box of within(group).getAllByRole("checkbox")) {
      if ((box as HTMLInputElement).checked) await user.click(box);
    }

    const submit = within(dialog).getByRole("button", {
      name: "Mint passport",
    });
    expect(submit).toBeDisabled();
    // Refused, and the reason is attached to the control rather than left in a
    // title no screen reader announces on a disabled button.
    const describedBy = submit.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(document.getElementById(describedBy ?? "")?.textContent).toMatch(
      /select at least one permission/i,
    );
  });

  it("shows the token once, moves focus to it, and does not close itself", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend());
    const dialog = await openMintDialog(user);
    await user.type(within(dialog).getByLabelText("Agent name"), "Scout");
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );

    const token = await within(dialog).findByText("mgp_live_0f3a91c4");
    // Focus lands on the region holding it: the token is disclosed exactly
    // once, so a reader whose focus stayed on the button would have to hunt
    // for what they just made.
    const region = token.closest(".passport-token");
    expect(region).toBeTruthy();
    expect(region).toHaveFocus();
    // Still open. Closing on success would take the only sight of the
    // credential with it.
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("announces a refused mint beside the button that produced it", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend({ refuse: true }));
    const dialog = await openMintDialog(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );

    const alert = await within(dialog).findByRole("alert");
    expect(alert).toHaveTextContent(/cannot lend that scope/i);
    // In the form, not in a band elsewhere on the card.
    expect(alert.closest("form")).toBeTruthy();
  });

  // An expired session is not a mint that quietly did nothing.
  //
  // The `me` probe is cached for five minutes, so a 401 here used to leave the
  // screen believing it was signed in: the button failed in silence and the
  // human had no way to learn why. That silence is what made the OAuth consent
  // screen's empty-passport guide inescapable — it sends you here to mint, the
  // mint 401s without saying so, and going back finds no passport and renders
  // the same guide again.
  //
  // Held here: the probe is re-run (so AuthGate re-evaluates and puts up the
  // login screen), every other cached answer is dropped (so the next contact to
  // sign in this tab is not shown this one's passport list), and the refusal is
  // still reported. The boundary's own rendering is AuthGate's contract and is
  // covered where AuthGate is.
  it("re-probes the session and drops other caches when the mint is unauthorized", async () => {
    const user = userEvent.setup();
    const backend = mintBackend({ expired: true });
    vi.stubGlobal("fetch", backend);

    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <SettingsScreen route={settingsAddress("agents")} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    // The ROW's verb opens the dialog; the dialog's submit is the plain label
    // clicked further down. This case renders its own client rather than going
    // through `openMintDialog`, because it reads the cache the mint is supposed to
    // drop.
    await user.click(
      await screen.findByRole("button", { name: "New passport" }),
    );
    const dialog = await screen.findByRole("dialog");

    // A cache entry from this session, which the next one must not inherit.
    client.setQueryData(["some-other-screen"], { secret: "previous session" });
    const meCallsBefore = backend.mock.calls.filter((call) =>
      String(call[0] instanceof Request ? call[0].url : call[0]).endsWith(
        "/v1/me",
      ),
    ).length;

    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );

    // The refusal is still shown where every other mint failure is shown.
    const alert = await within(dialog).findByRole("alert");
    expect(alert).toHaveTextContent(/session has expired/i);

    // The probe was re-run rather than left on its cached answer.
    await vi.waitFor(() => {
      const meCallsAfter = backend.mock.calls.filter((call) =>
        String(call[0] instanceof Request ? call[0].url : call[0]).endsWith(
          "/v1/me",
        ),
      ).length;
      expect(meCallsAfter).toBeGreaterThan(meCallsBefore);
    });

    // And nothing of this session is left for the next one to render.
    expect(client.getQueryData(["some-other-screen"])).toBeUndefined();
  });

  // The one that is about losing a credential rather than about layout.
  // `mint.reset()` detaches the observer; it does not cancel the request. A mint
  // closed mid-flight therefore still creates a passport on the server, and its
  // token — disclosed once, never re-served — goes with the closed dialog. So
  // the dialog refuses to close while the request is outstanding.
  it("cannot be closed while the mint is in flight", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend({ hang: true }));
    const dialog = await openMintDialog(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );

    const cancel = await within(dialog).findByRole("button", {
      name: "Cancel",
    });
    expect(cancel).toBeDisabled();
    expect(
      within(dialog).getByRole("button", { name: "Close" }),
    ).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("starts clean on re-open rather than showing the last mint's token", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend());
    const dialog = await openMintDialog(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );
    await within(dialog).findByText("mgp_live_0f3a91c4");
    await user.click(within(dialog).getByRole("button", { name: "Done" }));

    await screen.findByRole("button", { name: "New passport" });
    expect(screen.queryByRole("dialog")).toBeNull();
    await user.click(screen.getByRole("button", { name: "New passport" }));
    const reopened = await screen.findByRole("dialog");
    expect(reopened).toBeTruthy();
    expect(within(reopened).queryByText("mgp_live_0f3a91c4")).toBeNull();
    expect(within(reopened).getByLabelText("Agent name")).toHaveValue("");
  });
});

// What the anchor was told to save, read at the click: the module removes it
// straight after.
function captureDownloads(): string[] {
  const saved: string[] = [];
  Object.defineProperties(URL, {
    createObjectURL: { configurable: true, value: vi.fn(() => "blob:skill") },
    revokeObjectURL: { configurable: true, value: vi.fn() },
  });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    saved.push(this.download);
  });
  return saved;
}

function bundleRequests(backend: ReturnType<typeof vi.fn>): number {
  return backend.mock.calls.filter((call) =>
    String(call[0] instanceof Request ? call[0].url : call[0]).endsWith(
      "/v1/agent-bundle",
    ),
  ).length;
}

async function snippetOf(scope: HTMLElement) {
  return within(scope).findByTestId("passport-snippet");
}

describe("PassportCard — using a passport", () => {
  it("says what a passport is and offers both ways to use one", async () => {
    vi.stubGlobal("fetch", mintBackend());
    render("agents");
    expect(
      await screen.findByText(/a passport is your personal access token/i),
    ).toBeTruthy();
    expect(screen.getByText("In your AI tool")).toBeTruthy();
    expect(screen.getByText("In your own code")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Your passports" }),
    ).toBeTruthy();
    expect(screen.getByText(/listed under Connected MCP clients/)).toBeTruthy();
  });

  it("downloads the skill as margince-skill.zip with no passport minted", async () => {
    const user = userEvent.setup();
    const backend = mintBackend();
    vi.stubGlobal("fetch", backend);
    const saved = captureDownloads();
    render("agents");
    await user.click(
      await screen.findByRole("button", { name: "Download skill" }),
    );
    await vi.waitFor(() => expect(saved).toEqual(["margince-skill.zip"]));
    expect(bundleRequests(backend)).toBe(1);
  });

  it("says why the skill did not download", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", mintBackend({ bundle: "refuses" }));
    const saved = captureDownloads();
    render("agents");
    await user.click(
      await screen.findByRole("button", { name: "Download skill" }),
    );
    const cause = await screen.findByText(/MARGINCE_PUBLIC_BASE_URL/);
    expect(cause.closest('[role="alert"]')).toHaveTextContent(
      "Skill not downloaded",
    );
    expect(saved).toEqual([]);
  });

  it("switches the example's language and copies the code it shows", async () => {
    const user = userEvent.setup();
    const clipboard = stubClipboard("accepts");
    vi.stubGlobal("fetch", mintBackend());
    render("agents");
    const snippet = await snippetOf(document.body);
    expect(snippet).toHaveTextContent(`curl "${API_BASE}/companies?limit=5"`);

    await user.click(screen.getByRole("button", { name: "Python" }));
    expect(snippet).toHaveTextContent('os.environ["MARGINCE_PASSPORT"]');
    await user.click(screen.getByRole("button", { name: "JavaScript" }));
    expect(snippet).toHaveTextContent("process.env.MARGINCE_PASSPORT");
    expect(snippet).toHaveTextContent(`${API_BASE}/companies?limit=5`);

    await user.click(screen.getByRole("button", { name: "Copy example" }));
    await vi.waitFor(() => expect(clipboard.written).toHaveLength(1));
    expect(clipboard.written[0]).toBe(snippet.textContent);
    expect(clipboard.written[0]).toContain("fetch(");
  });

  it("copies the new passport, keeps it out of the live region and out of the example", async () => {
    const user = userEvent.setup();
    const clipboard = stubClipboard("accepts");
    vi.stubGlobal("fetch", mintBackend());
    const dialog = await openMintDialog(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );
    await within(dialog).findByText("mgp_live_0f3a91c4");

    const status = within(dialog).getByRole("status");
    expect(status).toHaveTextContent(/passport created/i);
    expect(status.textContent).not.toContain("mgp_");

    const snippet = await snippetOf(dialog);
    expect(snippet.textContent).toContain("$MARGINCE_PASSPORT");
    expect(snippet.textContent).not.toContain("mgp_");

    await user.click(
      within(dialog).getByRole("button", { name: "Copy passport" }),
    );
    await vi.waitFor(() =>
      expect(clipboard.written).toEqual(["mgp_live_0f3a91c4"]),
    );
  });

  it("downloads the skill from the dialog without closing it", async () => {
    const user = userEvent.setup();
    const backend = mintBackend();
    vi.stubGlobal("fetch", backend);
    const saved = captureDownloads();
    const dialog = await openMintDialog(user);
    await user.click(
      within(dialog).getByRole("button", { name: "Mint passport" }),
    );
    await within(dialog).findByText("mgp_live_0f3a91c4");
    await user.click(
      within(dialog).getByRole("button", { name: "Download skill" }),
    );
    await vi.waitFor(() => expect(saved).toEqual(["margince-skill.zip"]));
    expect(within(dialog).getByText("mgp_live_0f3a91c4")).toBeTruthy();
  });
});
