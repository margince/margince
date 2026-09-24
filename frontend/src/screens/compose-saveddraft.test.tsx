/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { messageText, writeMessage } from "../design-system/richtext-testing";
import { pickOption } from "../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

// The rep's own unsent message, kept by the server: what the composer reads
// back when it opens, writes when it closes, and hands the send to discard.

type MailDraft = components["schemas"]["MailDraft"];
type Sent = { key: string; query: string; body: unknown; headers: Headers };

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function problem(code: string, status: number) {
  return new Response(JSON.stringify({ code, title: "Conflict" }), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

const SAVED: MailDraft = {
  id: "md-1",
  anchor_type: "contact",
  anchor_id: "p-1",
  to: ["buyer@acme.test"],
  cc: [],
  bcc: [],
  subject: "Pricing for 40 seats",
  body: "Half written before lunch",
  html_body: "<p>Half written before lunch</p>",
  version: 3,
  created_at: "2026-09-20T09:00:00Z",
  updated_at: "2026-09-20T09:05:00Z",
};

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

type Route = () => Response | Promise<Response>;

function stubRoutes(overrides: Record<string, Route> = {}) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET" && method !== "DELETE") {
        body = request
          ? await request.clone().json()
          : JSON.parse(String(init?.body));
      }
      const headers = request
        ? request.headers
        : new Headers(init?.headers ?? {});
      sent.push({ key, query: url.search, body, headers });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /mail-drafts") return problem("not_found", 404);
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      if (isPreviewDoor(url.pathname)) {
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({});
    }),
  );
  return sent;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function composer(onClose: () => void, activityId?: string) {
  return (
    <ComposeModal
      activityId={activityId}
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
      open
      onClose={onClose}
    />
  );
}

const calls = (sent: Sent[], key: string) =>
  sent.filter((call) => call.key === key);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("a saved draft", () => {
  it("is read for the record the composer opened on and put back in the fields", async () => {
    const sent = stubRoutes({ "GET /mail-drafts": () => jsonResponse(SAVED) });
    render(composer(vi.fn()));

    expect(await screen.findByText("Saved draft restored")).toBeTruthy();
    expect(screen.getByDisplayValue("Pricing for 40 seats")).toBeTruthy();
    expect(messageText("Body")).toBe("Half written before lunch");
    expect(screen.getByText("buyer@acme.test")).toBeTruthy();
    const read = calls(sent, "GET /mail-drafts")[0];
    expect(new URLSearchParams(read?.query).get("anchor_type")).toBe("contact");
    expect(new URLSearchParams(read?.query).get("anchor_id")).toBe("p-1");
  });

  it("is read for the message a reply answers", async () => {
    const sent = stubRoutes();
    render(composer(vi.fn(), "act-1"));

    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    const query = new URLSearchParams(
      calls(sent, "GET /mail-drafts")[0]?.query,
    );
    expect(query.get("anchor_type")).toBe("activity");
    expect(query.get("anchor_id")).toBe("act-1");
  });

  it("saves over the version it restored and closes", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "PUT /mail-drafts": () => jsonResponse({ ...SAVED, version: 4 }),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    writeMessage("Body", "Finished after lunch");

    await userEvent.click(
      screen.getByRole("button", { name: "Save as draft" }),
    );

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const put = calls(sent, "PUT /mail-drafts")[0];
    expect(put?.headers.get("If-Match")).toBe("3");
    expect(put?.body).toEqual({
      anchor_type: "contact",
      anchor_id: "p-1",
      to: ["buyer@acme.test"],
      cc: [],
      bcc: [],
      subject: "Pricing for 40 seats",
      body: "Finished after lunch",
      html_body: "<p>Finished after lunch</p>",
    });
    expect(await screen.findByText("Draft saved")).toBeTruthy();
  });

  it("is written when the composer closes over words nobody saved", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "PUT /mail-drafts": () => jsonResponse(SAVED),
    });
    render(composer(onClose));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    // The close waits for the save, so a refused one cannot drop the text.
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const put = calls(sent, "PUT /mail-drafts")[0];
    expect(put?.headers.get("If-Match")).toBeNull();
    expect(put?.body).toMatchObject({ body: "Half written before lunch" });
  });

  it("is named restored when the composer reopens still holding it", async () => {
    const sent: Sent[] = stubRoutes({
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
      "PUT /mail-drafts": () => {
        const put = sent
          .filter((call) => call.key === "PUT /mail-drafts")
          .at(-1);
        return jsonResponse({ ...SAVED, ...(put?.body as object), version: 1 });
      },
    });
    function Page() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            Write again
          </button>
          <ComposeModal
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
            open={open}
            onClose={() => setOpen(false)}
          />
        </>
      );
    }
    render(<Page />);
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");
    await userEvent.click(
      screen.getByRole("button", { name: "Save as draft" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    await userEvent.click(screen.getByRole("button", { name: "Write again" }));

    expect(await screen.findByText("Saved draft restored")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Delete saved draft" }),
    ).toBeTruthy();
    expect(messageText("Body")).toBe("Half written before lunch");
    // The save's toast stands down, so the delete's confirmation is not
    // queued behind a Delete that has already been offered here.
    expect(screen.queryByText("Draft saved")).toBeNull();
    await userEvent.click(
      screen.getByRole("button", { name: "Delete saved draft" }),
    );
    expect(await screen.findByText("Saved draft deleted")).toBeTruthy();
  });

  it("is not written when the composer closes as it opened", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({ "GET /mail-drafts": () => jsonResponse(SAVED) });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("is not created by opening and closing an empty composer", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes();
    render(composer(onClose));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });

  it("can be deleted from the toast the close leaves behind", async () => {
    const sent = stubRoutes({
      "PUT /mail-drafts": () => jsonResponse(SAVED),
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
    });
    render(composer(vi.fn()));
    await waitFor(() =>
      expect(calls(sent, "GET /mail-drafts")).toHaveLength(1),
    );
    writeMessage("Body", "Half written before lunch");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await userEvent.click(
      await screen.findByRole("button", { name: "Delete" }),
    );

    expect(await screen.findByText("Saved draft deleted")).toBeTruthy();
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(1);
    expect(messageText("Body")).toBe("");
  });

  it("is deleted from the restored notice, and the fields empty", async () => {
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "DELETE /mail-drafts/md-1": () => new Response(null, { status: 204 }),
    });
    render(composer(vi.fn()));
    await screen.findByText("Saved draft restored");

    await userEvent.click(
      screen.getByRole("button", { name: "Delete saved draft" }),
    );

    await waitFor(() => expect(messageText("Body")).toBe(""));
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(1);
    expect(screen.queryByText("Saved draft restored")).toBeNull();
    expect(screen.queryByDisplayValue("Pricing for 40 seats")).toBeNull();
  });

  it("is never overwritten after another window moved it", async () => {
    const onClose = vi.fn();
    let reads = 0;
    const sent = stubRoutes({
      "GET /mail-drafts": () => {
        reads += 1;
        return jsonResponse(
          reads === 1
            ? SAVED
            : {
                ...SAVED,
                body: "Written in the other tab",
                html_body: "<p>Written in the other tab</p>",
                version: 5,
              },
        );
      },
      "PUT /mail-drafts": () => problem("version_skew", 409),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    writeMessage("Body", "Written here");

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(
      await screen.findByText("Draft changed in another window"),
    ).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
    expect(messageText("Body")).toBe("Written here");
    expect(calls(sent, "GET /mail-drafts")).toHaveLength(2);

    await userEvent.click(
      screen.getByRole("button", { name: "Load saved version" }),
    );
    expect(messageText("Body")).toBe("Written in the other tab");
  });

  it("keeps the composer open over a save that failed", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "PUT /mail-drafts": () =>
        new Response(JSON.stringify({ detail: "The draft store is down." }), {
          status: 503,
          headers: { "Content-Type": "application/problem+json" },
        }),
    });
    render(composer(onClose));
    writeMessage("Body", "Half written before lunch");

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(await screen.findByRole("alert")).toHaveProperty(
      "textContent",
      "The draft store is down.",
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(messageText("Body")).toBe("Half written before lunch");
  });

  it("is named to the send, which discards it", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(SAVED),
      "POST /emails": () => jsonResponse({ id: "act-9" }, 202),
    });
    render(composer(onClose));
    await screen.findByText("Saved draft restored");
    await pickOption(
      userEvent.setup(),
      screen.getByRole("combobox", { name: "Reason for contact" }),
      "Follow-up they requested",
    );

    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(calls(sent, "POST /emails")[0]?.body).toMatchObject({
      mail_draft_id: "md-1",
      body: "Half written before lunch",
    });
    // The server discarded it with the send; the composer asks for nothing.
    expect(calls(sent, "DELETE /mail-drafts/md-1")).toHaveLength(0);
    expect(calls(sent, "PUT /mail-drafts")).toHaveLength(0);
  });
});
