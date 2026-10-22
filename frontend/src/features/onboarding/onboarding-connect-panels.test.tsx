/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

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
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import {
  ImapConnectPanel,
  OAuthReturnPanel,
} from "./onboarding-connect-panels";

// The onboarding IMAP panel (G-10): the wizard's connect step for the one
// credential provider. It must post through the typed client, with the nested
// `{imap:{...}}` shape a standing connect expects. It must never use a raw
// fetch to the retired transient endpoint. A standing connect answers before
// any mail is read, so the panel hands back to the step. The step's account
// panel shows the mailbox from the roster.

function render(ui: ReactNode, client?: QueryClient) {
  return rtlRender(
    <QueryClientProvider
      client={
        client ??
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

async function fillValidForm() {
  await userEvent.clear(screen.getByLabelText("IMAP host"));
  await userEvent.type(screen.getByLabelText("IMAP host"), "mail.example.org");
  await userEvent.type(screen.getByLabelText("Email"), "lars@example.org");
  await userEvent.type(screen.getByLabelText("App password"), "app-password");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ImapConnectPanel", () => {
  it("connects IMAP through the typed client, not a raw fetch to a bespoke path", async () => {
    const calls: { url: string; body: unknown }[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push({ url: "POST /connectors/imap/connect", body });
        return jsonResponse({
          connection: {
            id: "c1",
            provider: "imap",
            status: "connected",
            scopes: [],
          },
        });
      },
    });
    const onDone = vi.fn();
    render(<ImapConnectPanel onDone={onDone} onDismiss={() => {}} />);
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].body).toMatchObject({
      imap: {
        host: "mail.example.org",
        port: 993,
        username: "lars@example.org",
        secret: "app-password",
        mailbox: "INBOX",
        max_messages: 30,
      },
    });

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
  });

  it("hands back to the step once the mailbox is live", async () => {
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse({
          connection: {
            id: "c1",
            provider: "imap",
            status: "connected",
            scopes: [],
          },
        }),
    });
    const onDone = vi.fn();
    render(<ImapConnectPanel onDone={onDone} onDismiss={() => {}} />);
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
  });

  it("surfaces a rejected IMAP login without echoing the host back", async () => {
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse(
          {
            code: "imap_login_rejected",
            detail: "The mailbox rejected these credentials.",
          },
          422,
        ),
    });
    render(<ImapConnectPanel onDone={vi.fn()} onDismiss={() => {}} />);
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    expect(
      await screen.findByText(/rejected these credentials/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/mail\.example\.org/)).not.toBeInTheDocument();
  });

  it("never retains the password after a failed submit", async () => {
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse({ code: "imap_unreachable", detail: "unreachable" }, 502),
    });
    render(<ImapConnectPanel onDone={vi.fn()} onDismiss={() => {}} />);
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    await screen.findByText(/could not be reached/i);
    expect(screen.getByLabelText("App password")).toHaveValue("");
  });

  it("invalidates the shared connectors query so Settings picks up the new connection immediately", async () => {
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse({
          connection: {
            id: "c1",
            provider: "imap",
            status: "connected",
            scopes: [],
          },
        }),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    render(<ImapConnectPanel onDone={vi.fn()} onDismiss={() => {}} />, client);
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["connectors"] }),
    );
  });

  // "Not now" closes THIS dialog without deciding anything — it never
  // contacts the server and never claims the whole required step as
  // skipped, which is a separate, more deliberate action the surface offers
  // beside the provider choice.
  it("dismisses without ever contacting the server or claiming the step skipped", async () => {
    const calls: unknown[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push(body);
        return jsonResponse({
          connection: {
            id: "c1",
            provider: "imap",
            status: "connected",
            scopes: [],
          },
        });
      },
    });
    const onDone = vi.fn();
    const onDismiss = vi.fn();
    render(<ImapConnectPanel onDone={onDone} onDismiss={onDismiss} />);
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
    expect(onDone).not.toHaveBeenCalled();
    expect(calls.length).toBe(0);
  });

  // A successful connect that lands after the reader already backed out
  // would leave a mailbox connected against a "no" the panel already
  // promised — so dismissal has to wait for the in-flight POST to settle.
  it("cannot be dismissed while the connect POST is still in flight", async () => {
    // A box, not a bare `let`: TS's control-flow narrowing otherwise loses
    // the function type across the callback boundary that assigns it.
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    const onDismiss = vi.fn();
    const onDone = vi.fn();
    render(<ImapConnectPanel onDone={onDone} onDismiss={onDismiss} />);
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Not now" })).toBeDisabled(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));
    expect(onDismiss).not.toHaveBeenCalled();

    // A success that lands after the (blocked) dismiss attempt hands back to
    // the step, never to the dismissal.
    deferred.resolve?.(
      jsonResponse({
        connection: {
          id: "c1",
          provider: "imap",
          status: "connected",
          scopes: [],
        },
      }),
    );
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    expect(onDismiss).not.toHaveBeenCalled();
  });

  // The disabled "Not now" button only guards ITS OWN click — a caller
  // wrapping this panel in a dialog also has to keep that dialog's X,
  // Escape, and backdrop from closing over the same in-flight POST, and this
  // is the one signal that lets it.
  it("reports its in-flight state so a caller can guard the dialog's other dismissal routes too", async () => {
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    const pendingChanges: boolean[] = [];
    render(
      <ImapConnectPanel
        onDone={vi.fn()}
        onDismiss={() => {}}
        onPendingChange={(pending) => pendingChanges.push(pending)}
      />,
    );
    await fillValidForm();
    await userEvent.click(
      screen.getByRole("button", { name: /test and connect/i }),
    );
    await waitFor(() => expect(pendingChanges).toContain(true));
    deferred.resolve?.(
      jsonResponse({
        connection: {
          id: "c1",
          provider: "imap",
          status: "connected",
          scopes: [],
        },
      }),
    );
    await waitFor(() => expect(pendingChanges.at(-1)).toBe(false));
  });
});

// A granted mail leg is confirmed from the roster. The exit back to the ask is
// offered only once the roster has answered that nothing came back.
describe("OAuthReturnPanel confirming a granted mailbox", () => {
  it("keeps the plain exit when no connection could be confirmed", async () => {
    installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
    render(<OAuthReturnPanel outcome="ok" onDone={vi.fn()} />);

    await screen.findByText("The connection could not be confirmed.");
    expect(screen.getByRole("button", { name: /^done$/i })).toBeInTheDocument();
  });

  // A roster that never answered is not evidence of anything. `live` is
  // `undefined` for a failed query exactly as it is for an empty one, so
  // without this gate a transient 500 records the step as deliberately
  // skipped — a choice the reader did not make, on the strength of a request
  // that came back with nothing to say. The stage's own exit stays available;
  // it just belongs to the reader.
  it("withholds the Done escape when the roster query failed", async () => {
    installFetchStub({
      "GET /connectors": () => jsonResponse({ title: "boom" }, 500),
    });
    render(<OAuthReturnPanel outcome="ok" onDone={vi.fn()} />);

    await screen.findByText("The connection could not be confirmed.");
    expect(
      screen.queryByRole("button", { name: /^done$/i }),
    ).not.toBeInTheDocument();
  });

  // Without this gate the escape reads on `live === undefined`, which is also
  // true of the roster's very first render — a reader could click straight
  // through before the OAuth connection was ever confirmed live.
  it("withholds the Done escape while the roster is still verifying", async () => {
    // A box, not a bare `let`: TS's control-flow narrowing otherwise loses
    // the function type across the callback boundary that assigns it.
    const deferred: { resolve: ((r: Response) => void) | null } = {
      resolve: null,
    };
    installFetchStub({
      "GET /connectors": () =>
        new Promise((resolve) => {
          deferred.resolve = resolve;
        }),
    });
    render(<OAuthReturnPanel outcome="ok" onDone={vi.fn()} />);

    await screen.findByText("Confirming connection…");
    expect(
      screen.queryByRole("button", { name: /^done$/i }),
    ).not.toBeInTheDocument();

    deferred.resolve?.(jsonResponse({ data: [] }));
    expect(
      await screen.findByRole("button", { name: /^done$/i }),
    ).toBeInTheDocument();
  });
});

describe("OAuthReturnPanel reading a reader-typed outcome", () => {
  // The permanent-failure table is a plain object literal indexed with the
  // outcome segment off the address bar. `constructor`, `toString` and their
  // siblings are INHERITED members, so a bare index answers them with a
  // function rather than undefined — truthy, which routes this panel into the
  // permanent-failure banner and then asks the catalogue for a message keyed by
  // that function. The reader gets advice about credentials nobody refused,
  // with no way back, where the retry they need was one segment away.
  it.each([
    "constructor",
    "toString",
    "__proto__",
    "valueOf",
    "hasOwnProperty",
  ])("offers the retry, not a permanent failure, for %s", async (outcome) => {
    installFetchStub({ "GET /connectors": () => jsonResponse({ data: [] }) });
    render(<OAuthReturnPanel outcome={outcome} onDone={vi.fn()} />);

    expect(
      await screen.findByText("The connection could not be confirmed."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Connect again in Settings under Connections."),
    ).toBeInTheDocument();
  });
});
