/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { ImapConnectForm } from "./imap-connect-form";

// The inline IMAP connect form (Task 6): first-connect and reconnect for the
// one credential provider, done through the typed client — never a raw
// fetch — and never claiming a connection the server hasn't confirmed.

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

async function fillValidForm(user: UserEvent) {
  await user.type(screen.getByLabelText("IMAP server *"), "mail.example.org");
  await user.type(screen.getByLabelText("Email address *"), "lars@example.org");
  await user.type(screen.getByLabelText("App password *"), "app-password");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ImapConnectForm", () => {
  it("posts the imap block through the typed client, not a raw fetch", async () => {
    const user = userEvent.setup();
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
    const onConnected = vi.fn();
    render(
      <ImapConnectForm open onClose={() => {}} onConnected={onConnected} />,
    );
    await fillValidForm(user);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0].body).toMatchObject({
      imap: {
        host: "mail.example.org",
        port: 993,
        username: "lars@example.org",
        secret: "app-password",
        mailbox: "INBOX",
        max_messages: 50,
      },
    });
    // Sending is opt-in: left unticked, the connection stays read-only.
    expect(calls[0].body).not.toHaveProperty("imap.smtp");
    await waitFor(() => expect(onConnected).toHaveBeenCalled());
  });

  it("surfaces a rejected login without echoing the host back", async () => {
    const user = userEvent.setup();
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
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(
      await screen.findByText(/rejected these credentials/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/mail\.example\.org/)).not.toBeInTheDocument();
  });

  it("surfaces an unreachable server as a host/port problem", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse(
          {
            code: "imap_unreachable",
            detail:
              "The mail server could not be reached. Check the host and port.",
          },
          502,
        ),
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(
      await screen.findByText(/could not be reached/i),
    ).toBeInTheDocument();
  });

  // Connect is always pressable, and pressing it early names what is missing
  // at the fields and beside the button rather than leaving a grey button to
  // explain itself. Nothing is posted until the form can be dialled.
  it("names what is still needed when Connect is pressed early", async () => {
    const user = userEvent.setup();
    const calls: unknown[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push(body);
        return jsonResponse({}, 500);
      },
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(
      screen.getByText("Required: IMAP server, Email address, App password"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Required fields")).toHaveLength(3);
    expect(calls).toHaveLength(0);
  });

  it("names an out-of-range port instead of ignoring the press", async () => {
    const user = userEvent.setup();
    const calls: unknown[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push(body);
        return jsonResponse({}, 500);
      },
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    const port = screen.getByLabelText("Port");
    await user.clear(port);
    await user.type(port, "70000");
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(screen.getByText("Out of range: Port")).toBeInTheDocument();
    expect(screen.getByText("From 1 to 65535")).toBeInTheDocument();
    expect(calls).toHaveLength(0);
  });

  it("never retains the secret after a failed submit", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse({ code: "imap_unreachable", detail: "unreachable" }, 502),
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await screen.findByText(/could not be reached/i);
    expect(screen.getByLabelText("App password *")).toHaveValue("");
  });

  it("turns sending on with the outgoing server's settings and their defaults", async () => {
    const user = userEvent.setup();
    const calls: unknown[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push(body);
        return jsonResponse({
          connection: { id: "c1", provider: "imap", status: "connected" },
        });
      },
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByLabelText(/Send from this mailbox/));
    // An empty server field falls back to the IMAP server, which it shows.
    expect(screen.getByLabelText("SMTP server")).toHaveAttribute(
      "placeholder",
      "mail.example.org",
    );
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]).toMatchObject({
      imap: { smtp: { port: 465, security: "tls" } },
    });
    expect(calls[0]).not.toHaveProperty("imap.smtp.host");
  });

  it("sends a typed outgoing server and port as written", async () => {
    const user = userEvent.setup();
    const calls: unknown[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push(body);
        return jsonResponse({
          connection: { id: "c1", provider: "imap", status: "connected" },
        });
      },
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByLabelText(/Send from this mailbox/));
    await user.type(screen.getByLabelText("SMTP server"), " smtp.example.org ");
    await user.type(screen.getByLabelText("SMTP port"), "2525");
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(calls.length).toBe(1));
    expect(calls[0]).toMatchObject({
      imap: { smtp: { host: "smtp.example.org", port: 2525 } },
    });
  });

  it("names an out-of-range SMTP port only while sending is on", async () => {
    const user = userEvent.setup();
    const calls: unknown[] = [];
    installFetchStub({
      "POST /connectors/imap/connect": (body) => {
        calls.push(body);
        return jsonResponse({}, 500);
      },
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByLabelText(/Send from this mailbox/));
    await user.type(screen.getByLabelText("SMTP port"), "0");
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(screen.getByText("Out of range: SMTP port")).toBeInTheDocument();
    expect(calls).toHaveLength(0);
  });

  it("names a refused outgoing login apart from a refused mailbox login", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse({ code: "smtp_login_rejected", detail: "rejected" }, 422),
    });
    render(<ImapConnectForm open onClose={() => {}} />);
    await fillValidForm(user);
    await user.click(screen.getByLabelText(/Send from this mailbox/));
    await user.click(screen.getByRole("button", { name: "Connect" }));
    expect(
      await screen.findByText(/The SMTP server rejected this login/),
    ).toBeInTheDocument();
  });

  it("invalidates the shared connectors query so every roster shows the new connection", async () => {
    const user = userEvent.setup();
    installFetchStub({
      "POST /connectors/imap/connect": () =>
        jsonResponse({
          connection: { id: "c1", provider: "imap", status: "connected" },
        }),
    });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(client, "invalidateQueries");
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ImapConnectForm open onClose={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await fillValidForm(user);
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ["connectors"] }),
    );
  });
});
