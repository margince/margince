/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { stubClipboard } from "../design-system/clipboard-testing";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "../testing/company.fixtures";
import { meFixture } from "../testing/mefixture";
import {
  OAuthAppDrawer,
  RedirectAddressesDialog,
  RemoveOAuthAppDialog,
} from "./oauth-app-drawer";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const CLIENT_ID = "111-abc.apps.googleusercontent.com";

type OAuthAppResponse = {
  provider: "google" | "microsoft";
  configured: boolean;
  client_id: string;
  tenant?: string;
  source: "stored" | "environment" | "none";
  locks?: {
    key: string;
    origin: { source: "env"; name: string };
  }[];
  redirect_uris: { purpose: string; url: string }[];
};

// The three sources an app can have. `configured` alone cannot tell them apart:
// it is true for both a stored app and one the deployment supplies.
const SIGN_IN_URI = "https://api.acme.test/v1/auth/oidc/google/callback";
const CONNECT_URI = "https://api.acme.test/v1/connectors/google/callback";

function stored(): OAuthAppResponse {
  return {
    provider: "google",
    configured: true,
    client_id: CLIENT_ID,
    source: "stored",
    redirect_uris: [
      { purpose: "sign_in", url: SIGN_IN_URI },
      { purpose: "mailbox_connect", url: CONNECT_URI },
    ],
  };
}

function fromEnvironment(): OAuthAppResponse {
  return {
    provider: "google",
    configured: true,
    client_id: CLIENT_ID,
    source: "environment",
    locks: [
      {
        key: "capture.google_app",
        origin: { source: "env", name: "MARGINCE_GMAIL_CLIENT_ID" },
      },
    ],
    redirect_uris: [{ purpose: "mailbox_connect", url: CONNECT_URI }],
  };
}

function absent(): OAuthAppResponse {
  return {
    provider: "google",
    configured: false,
    client_id: "",
    source: "none",
    redirect_uris: [],
  };
}

function mount(
  app: OAuthAppResponse,
  provider: "google" | "microsoft" = "google",
  options: Readonly<{
    view?: "drawer" | "remove" | "redirects";
    canManage?: boolean;
    refuseWrites?: boolean;
    /** A write answers only once this settles, so a test can act mid-save. */
    holdWrites?: Promise<void>;
  }> = {},
) {
  const calls: { method: string; url: string; body?: unknown }[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    const url = new URL(request.url).pathname;
    if (request.method === "GET") {
      if (url.endsWith("/me")) {
        // Through the fixture rather than a hand-built body. The grant shape is
        // the capability layer's, so a test that spelled its own would prove
        // nothing about what the real /me returns.
        return jsonResponse(
          meFixture({
            allow: {
              capture_settings:
                options.canManage === false ? ["read"] : ["read", "update"],
            },
          }),
        );
      }
      return jsonResponse(app);
    }
    calls.push({
      method: request.method,
      url,
      body: request.body ? JSON.parse(await request.text()) : undefined,
    });
    await options.holdWrites;
    if (options.refuseWrites) {
      return jsonResponse(
        { title: "Unprocessable", status: 422, detail: "client_id invalid" },
        422,
      );
    }
    return new Response(null, { status: 204 });
  });
  vi.stubGlobal("fetch", fetchMock);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const onClose = vi.fn();
  const views = {
    drawer: OAuthAppDrawer,
    remove: RemoveOAuthAppDialog,
    redirects: RedirectAddressesDialog,
  };
  const View = views[options.view ?? "drawer"];
  render(<View provider={provider} open onClose={onClose} />, {
    wrapper: Wrap,
  });
  return { calls, qc, onClose };
}

// The reads an app write changes besides its own. The connector roster decides
// availability from the app, and the sign-in policy decides whether the
// provider's button can show. Seeded so a test can ask if a write marked them
// stale.
const DEPENDENTS = [["connectors"], ["authentication-policy"]];

function seedDependents(qc: QueryClient) {
  for (const key of DEPENDENTS) {
    qc.setQueryData(key, {});
  }
}

function dependentsRefreshed(qc: QueryClient): boolean {
  return DEPENDENTS.every(
    (key) => qc.getQueryState(key)?.isInvalidated === true,
  );
}

// Removing a stored app, from its row's menu. The secret cannot be read back
// and every mailbox on the vendor connects through the app, so a stray click
// must not be enough.
describe("removing an app", () => {
  const PHRASE = "I am aware of the risk";

  it("deletes nothing until the phrase is typed and the removal confirmed", async () => {
    const user = userEvent.setup();
    const { calls } = mount(stored(), "google", { view: "remove" });
    const remove = await screen.findByRole("button", { name: "Remove app" });
    expect(remove).toHaveProperty("disabled", true);
    await user.type(screen.getByRole("textbox"), "I am aware");
    expect(remove).toHaveProperty("disabled", true);
    expect(calls).toEqual([]);
  });

  it("says what removing it changes before anything is typed", async () => {
    mount(stored(), "google", { view: "remove" });
    expect(
      await screen.findByText(
        "Gmail and Google Calendar connections use this app.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("First-run setup asks for an app again."),
    ).toBeTruthy();
  });

  // Onboarding tiles and the sign-in live line read these, and both went stale
  // after a removal until the page was reloaded.
  it("removes the app, closes, and refreshes what the app decides", async () => {
    const user = userEvent.setup();
    const { calls, qc, onClose } = mount(stored(), "google", {
      view: "remove",
    });
    seedDependents(qc);
    await user.type(await screen.findByRole("textbox"), PHRASE);
    await user.click(screen.getByRole("button", { name: "Remove app" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls).toEqual([
      {
        method: "DELETE",
        url: "/v1/installation/oauth-apps/google",
        body: undefined,
      },
    ]);
    await waitFor(() => expect(dependentsRefreshed(qc)).toBe(true));
  });

  // A refused DELETE leaves the dialog open, so its reason is said inside it
  // rather than behind the modal.
  it("keeps the dialog open with the reason when the server refuses", async () => {
    const user = userEvent.setup();
    const { onClose } = mount(stored(), "google", {
      view: "remove",
      refuseWrites: true,
    });
    await user.type(await screen.findByRole("textbox"), PHRASE);
    await user.click(screen.getByRole("button", { name: "Remove app" }));

    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText(/client_id invalid/)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });
});

// The table an operator copies from into the vendor's console. Drawn through
// the setup drawer, the surface it is read on.
describe("the redirect address table", () => {
  // A copy control that returned on a missing clipboard did nothing on plain
  // http, and an empty address got pasted.
  it("says the clipboard refused a row rather than leaving a dead button", async () => {
    const user = userEvent.setup();
    mount(stored(), "google", { view: "redirects" });
    await screen.findByText(SIGN_IN_URI);
    stubClipboard("absent");

    await user.click(screen.getByRole("button", { name: "Copy Sign-in URI" }));

    expect(await screen.findByText(/clipboard access denied/i)).toBeTruthy();
    expect(screen.getByText(/copy it manually/i)).toBeTruthy();
  });

  // The clipboard holds one address, so only the row copied last reads Copied.
  it("lets only the row the clipboard actually holds say it was copied", async () => {
    const user = userEvent.setup();
    mount(stored(), "google", { view: "redirects" });
    await screen.findByText(SIGN_IN_URI);
    const clipboard = stubClipboard("accepts");

    await user.click(screen.getByRole("button", { name: "Copy Sign-in URI" }));
    await screen.findByRole("button", { name: "Copied Sign-in URI" });
    await user.click(screen.getByRole("button", { name: "Copy Mailbox URI" }));

    await waitFor(() =>
      expect(screen.getAllByRole("button", { name: /^Copied / })).toHaveLength(
        1,
      ),
    );
    expect(
      screen.getByRole("button", { name: "Copy Sign-in URI" }),
    ).toBeTruthy();
    expect(clipboard.written).toEqual([SIGN_IN_URI, CONNECT_URI]);
  });

  it("does not advertise an address for a flow this deployment does not serve", async () => {
    mount(fromEnvironment(), "google", { view: "redirects" });
    expect(await screen.findByText(CONNECT_URI)).toBeTruthy();
    expect(screen.queryByText(SIGN_IN_URI)).toBeNull();
  });

  it("names each address by what it is used for", async () => {
    mount(stored(), "google", { view: "drawer" });
    await screen.findByText(SIGN_IN_URI);
    const table = screen.getByRole("table");
    expect(within(table).getByText("Mailbox")).toBeTruthy();
    expect(within(table).getByText("Sign-in")).toBeTruthy();
  });
});

describe("the app setup drawer", () => {
  const ENTRA_ID = "11111111-2222-3333-4444-555555555555";
  const DIRECTORY = "99999999-8888-7777-6666-555555555555";

  // The fields stay locked until the reader's grants are known, so a test
  // types only once they open.
  async function openFields(): Promise<HTMLInputElement> {
    const id = await screen.findByLabelText<HTMLInputElement>("Client ID");
    await waitFor(() => expect(id.disabled).toBe(false));
    return id;
  }

  function pinnedMicrosoft(): OAuthAppResponse {
    return {
      provider: "microsoft",
      configured: true,
      client_id: ENTRA_ID,
      tenant: DIRECTORY,
      source: "stored",
      redirect_uris: [
        {
          purpose: "mailbox_connect",
          url: "https://api.acme.test/v1/connectors/graph/callback",
        },
      ],
    };
  }

  it("saves the trimmed pair, then closes and refreshes what the app decides", async () => {
    const user = userEvent.setup();
    const { calls, qc, onClose } = mount(absent(), "google", {
      view: "drawer",
    });
    seedDependents(qc);
    await user.type(await openFields(), ` ${CLIENT_ID} `);
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-secret ");
    await user.click(screen.getByRole("button", { name: "Save Google app" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls).toEqual([
      {
        method: "PUT",
        url: "/v1/installation/oauth-apps/google",
        body: { client_id: CLIENT_ID, client_secret: "GOCSPX-secret" },
      },
    ]);
    await waitFor(() => expect(dependentsRefreshed(qc)).toBe(true));
  });

  it("cannot be dismissed while the save is on its way", async () => {
    const user = userEvent.setup();
    let answer = () => {};
    const held = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const { calls, onClose } = mount(absent(), "google", {
      view: "drawer",
      holdWrites: held,
    });
    await user.type(await openFields(), CLIENT_ID);
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-secret");
    await user.click(screen.getByRole("button", { name: "Save Google app" }));
    await waitFor(() => expect(calls.length).toBe(1));

    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();

    answer();
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  });

  it("will not save until both halves of the pair are there", async () => {
    const user = userEvent.setup();
    mount(absent(), "google", { view: "drawer" });
    const id = await openFields();
    const save = screen.getByRole("button", { name: "Save Google app" });
    expect(save).toHaveProperty("disabled", true);
    await user.type(id, CLIENT_ID);
    expect(save).toHaveProperty("disabled", true);
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-secret");
    expect(save).toHaveProperty("disabled", false);
  });

  // Replacing a stored app is usually a secret rotation. The drawer opens with
  // the stored client id and directory in place. The rotation is one field and
  // never widens a pinned app to every company.
  it("opens a stored app with its client id and directory, and sends both back", async () => {
    const user = userEvent.setup();
    const { calls } = mount(pinnedMicrosoft(), "microsoft", {
      view: "drawer",
    });
    const id = await screen.findByLabelText<HTMLInputElement>("Client ID");
    await waitFor(() => expect(id.value).toBe(ENTRA_ID));
    expect(
      screen.getByLabelText<HTMLInputElement>("Directory (tenant) ID").value,
    ).toBe(DIRECTORY);
    await user.type(screen.getByLabelText("Client secret"), "rotated");
    await user.click(
      screen.getByRole("button", { name: "Save Microsoft app" }),
    );

    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]?.body).toEqual({
      client_id: ENTRA_ID,
      client_secret: "rotated",
      tenant: DIRECTORY,
    });
  });

  // The installation's app refuses every value, so the form is locked, says
  // where the app is set and offers no Save that would be ignored.
  it("locks the form when this installation's configuration supplies the app", async () => {
    mount(fromEnvironment(), "google", { view: "drawer" });
    await screen.findByText(CONNECT_URI);
    const clientId = screen.getByLabelText<HTMLInputElement>("Client ID");
    expect(clientId.value).toBe("");
    expect(clientId.disabled).toBe(true);
    const reason = screen.getByText(
      "Set by MARGINCE_GMAIL_CLIENT_ID (environment). Change it there.",
    );
    expect(clientId.getAttribute("aria-describedby")?.split(" ")).toContain(
      reason.id,
    );
    expect(
      screen.getByRole("button", { name: "Save Google app" }),
    ).toHaveProperty("disabled", true);
  });

  it("keeps the drawer open with the reason when the server refuses the pair", async () => {
    const user = userEvent.setup();
    const { onClose } = mount(absent(), "google", {
      view: "drawer",
      refuseWrites: true,
    });
    await user.type(await openFields(), CLIENT_ID);
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-secret");
    await user.click(screen.getByRole("button", { name: "Save Google app" }));

    expect(await screen.findByText("App not saved")).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("tells a reader who cannot set up company apps why Save is refused", async () => {
    mount(absent(), "google", { view: "drawer", canManage: false });
    expect(
      await screen.findByText("Only an admin can set up company apps"),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Save Google app" }),
    ).toHaveProperty("disabled", true);
  });

  // Widening a pinned app takes an explicit act. An empty field means "not
  // pinned" and the server refuses a tenant of "", so the field is omitted
  // rather than sent blank.
  it("widens a pinned app only when the directory is cleared", async () => {
    const user = userEvent.setup();
    const { calls } = mount(pinnedMicrosoft(), "microsoft", {
      view: "drawer",
    });
    const directory = await screen.findByLabelText<HTMLInputElement>(
      "Directory (tenant) ID",
    );
    await waitFor(() => expect(directory.value).toBe(DIRECTORY));
    await user.type(screen.getByLabelText("Client secret"), "s3cret");
    await user.clear(directory);
    await user.click(
      screen.getByRole("button", { name: "Save Microsoft app" }),
    );

    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]?.body).not.toHaveProperty("tenant");
  });

  it("asks for a directory only for Microsoft", async () => {
    mount(absent(), "google", { view: "drawer" });
    await screen.findByLabelText("Client ID");
    expect(screen.queryByLabelText("Directory (tenant) ID")).toBeNull();
  });
});
