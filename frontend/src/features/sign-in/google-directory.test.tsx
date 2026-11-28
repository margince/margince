/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { GoogleDirectoryPanel } from "./google-directory";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const KEY_FILE = JSON.stringify({
  type: "service_account",
  client_email: "reader@example-project.iam.gserviceaccount.com",
  private_key: "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The status a GET answers, and every write the panel makes.
function mount(
  status: Record<string, unknown>,
  put: () => Response = () => jsonResponse(status),
  canManage = true,
) {
  const writes: { method: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "GET") {
        return jsonResponse(status);
      }
      const text = await request.text();
      writes.push({
        method: request.method,
        body: text === "" ? null : JSON.parse(text),
      });
      return request.method === "PUT"
        ? put()
        : new Response(null, { status: 204 });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  render(
    <GoogleDirectoryPanel
      canRead
      canManage={canManage}
      lock={{ disabled: false }}
    />,
    { wrapper: Wrap },
  );
  return { writes };
}

describe("the Google group lookup panel", () => {
  it("says members get nothing from the map while no credential is stored", async () => {
    mount({ configured: false });
    expect(
      await screen.findByText(englishCopy["googleDirectory.state.none"]),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["googleDirectory.remove"],
      }),
    ).toBeNull();
  });

  it("names the failure class and what it costs when the last lookup failed", async () => {
    mount({
      configured: true,
      last_result: "failed",
      last_reason: "delegation_refused",
      last_checked_at: "2026-11-20T10:00:00Z",
    });
    expect(
      await screen.findByText(/Google refused the delegation\./),
    ).toBeTruthy();
    expect(
      screen.getByText(/Nobody loses a role they already hold\./),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: englishCopy["googleDirectory.remove"],
      }),
    ).toBeTruthy();
  });

  it("sends the key file and the admin address, and nothing else, to be checked and stored", async () => {
    const user = userEvent.setup();
    const { writes } = mount({ configured: false });
    await screen.findByText(/Not connected\./);
    const save = screen.getByRole("button", {
      name: englishCopy["googleDirectory.save"],
    });
    expect(
      save.hasAttribute("disabled") ||
        save.getAttribute("aria-disabled") === "true",
    ).toBe(true);

    await user.click(
      screen.getByLabelText(englishCopy["serviceAccountKey.label"]),
    );
    await user.paste(KEY_FILE);
    await user.click(
      screen.getByLabelText(englishCopy["googleDirectory.admin"]),
    );
    await user.paste("admin@example.test");
    await user.click(
      screen.getByRole("button", { name: englishCopy["googleDirectory.save"] }),
    );

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toEqual({
      method: "PUT",
      body: {
        service_account_json: KEY_FILE,
        admin_subject: "admin@example.test",
      },
    });
  });

  it("shows the server's refusal and keeps the typed key so it can be corrected", async () => {
    const user = userEvent.setup();
    mount({ configured: false }, () =>
      jsonResponse(
        {
          type: "about:blank",
          title: "Unprocessable",
          status: 422,
          code: "validation_error",
          detail: "Google refused the delegation.",
        },
        422,
      ),
    );
    await screen.findByText(/Not connected\./);
    await user.click(
      screen.getByLabelText(englishCopy["serviceAccountKey.label"]),
    );
    await user.paste(KEY_FILE);
    await user.click(
      screen.getByLabelText(englishCopy["googleDirectory.admin"]),
    );
    await user.paste("admin@example.test");
    await user.click(
      screen.getByRole("button", { name: englishCopy["googleDirectory.save"] }),
    );

    expect(
      await screen.findByText(/Google refused the delegation\./),
    ).toBeTruthy();
    expect(
      (
        screen.getByLabelText(
          englishCopy["serviceAccountKey.label"],
        ) as HTMLTextAreaElement
      ).value,
    ).toBe(KEY_FILE);
  });

  it("removes the connection", async () => {
    const user = userEvent.setup();
    const { writes } = mount({ configured: true });
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["googleDirectory.remove"],
      }),
    );
    await waitFor(() =>
      expect(writes).toEqual([{ method: "DELETE", body: null }]),
    );
  });

  it("shows the status and no form to a reader who may not change it", async () => {
    mount(
      {
        configured: true,
        last_result: "ok",
        last_checked_at: "2026-11-20T10:00:00Z",
      },
      undefined,
      false,
    );
    expect(
      await screen.findByText(/Connected\. The last lookup worked/),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["googleDirectory.save"],
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["googleDirectory.remove"],
      }),
    ).toBeNull();
  });
});
