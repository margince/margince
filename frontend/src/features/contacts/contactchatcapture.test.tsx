/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ContactChatCapture } from "./contactchatcapture";

type Reading = components["schemas"]["AttachmentExtraction"];

const READ_TEXT = "Lena: Are we still on for Thursday?\nMe: Yes, 10am.";

const DONE: Reading = {
  id: "read-1",
  question: "chat_capture",
  status: "done",
  stalled: false,
  fields: [
    {
      field: "chat_text",
      value: READ_TEXT,
      source_quote: "Lena: Are we still on for Thursday?",
      page_or_section: "messages",
      confidence: "high",
    },
    {
      field: "counterparty_name",
      value: "Lena Hoffmann",
      source_quote: "Lena Hoffmann",
      page_or_section: "header",
      confidence: "high",
    },
  ],
  omitted: [{ field: "counterparty_phone", reason: "not_stated_in_file" }],
  created_at: "2026-11-05T09:00:00Z",
};

const UNREAD: Reading = {
  ...DONE,
  status: "failed",
  status_detail:
    "this installation has no AI model configured for reading documents",
  fields: [],
  omitted: [],
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

type Sent = { method: string; path: string; body: unknown };

// Answers the capture's requests by method and path, and records each one, so
// a test can say which writes happened and in what words.
function stubServer(reading: Reading) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const request =
        input instanceof Request
          ? input
          : new Request(new URL(input, "http://localhost"), init);
      const { pathname } = new URL(request.url);
      const text = request.headers.get("content-type")?.includes("json")
        ? await request.text()
        : "";
      sent.push({
        method: request.method,
        path: pathname,
        body: text ? JSON.parse(text) : null,
      });
      if (pathname === "/v1/channel-providers") {
        return json({
          data: [
            {
              provider: "whatsapp",
              label: "WhatsApp",
              credential_model: "workspace_bot",
              supplies_transport: false,
              attachments: { max_bytes: 1, max_count: 1 },
            },
          ],
        });
      }
      if (request.method === "POST" && pathname === "/v1/attachments") {
        return json(
          {
            id: "att-1",
            entity_type: "contact",
            entity_id: "c-1",
            filename: "chat.png",
            source: "upload",
            captured_by: "human:u-1",
            created_at: "2026-11-05T09:00:00Z",
          },
          201,
        );
      }
      // Only the start answers on the bare path. A poll of the newest reading
      // there would read whatever was started last, not this panel's reading.
      if (
        request.method === "POST" &&
        pathname === "/v1/attachments/att-1/extraction"
      ) {
        return json({ id: "read-1", status: "queued", joined: false }, 202);
      }
      if (pathname === "/v1/attachments/att-1/extractions/read-1") {
        return json(reading);
      }
      if (pathname === "/v1/attachments/att-1/extraction:file-chat") {
        return json({ activity_id: "act-1" });
      }
      if (request.method === "DELETE" && pathname === "/v1/attachments/att-1") {
        return new Response(null, { status: 204 });
      }
      return json({ title: "unexpected" }, 500);
    }),
  );
  return sent;
}

function show(onClose = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactChatCapture
          contactId="c-1"
          contactName="Lena Hoffmann"
          onClose={onClose}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return onClose;
}

async function pickScreenshot() {
  const user = userEvent.setup();
  await user.upload(
    screen.getByLabelText(en["chatCapture.drop"]),
    new File(["png"], "chat.png", { type: "image/png" }),
  );
  return user;
}

const fileButton = () =>
  screen.findByRole("button", { name: en["chatCapture.file"] });

const filings = (sent: Sent[]) =>
  sent.filter((s) => s.path.endsWith("extraction:file-chat"));

// The text the one filing sent, or undefined when nothing was filed.
function filedBody(sent: Sent[]): string | undefined {
  const body = filings(sent)[0]?.body;
  return body ? (body as { body: string }).body : undefined;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("capturing a chat from a screenshot", () => {
  it("reads the screenshot for a chat and files nothing until the reader confirms", async () => {
    const sent = stubServer(DONE);
    const onClose = show();
    const user = await pickScreenshot();

    const text = await screen.findByLabelText(en["chatCapture.text"]);
    expect((text as HTMLTextAreaElement).value).toBe(READ_TEXT);
    expect(screen.getByText(en["chatCapture.readBadge"])).toBeTruthy();
    expect(screen.getByText(/Lena Hoffmann/)).toBeTruthy();
    expect(
      sent.find(
        (s) =>
          s.path === "/v1/attachments/att-1/extraction" && s.method === "POST",
      )?.body,
    ).toEqual({ question: "chat_capture" });
    expect(filings(sent)).toHaveLength(0);

    await user.click(await fileButton());

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(filings(sent)[0]?.body).toEqual({
      extraction_id: "read-1",
      channel_provider: "whatsapp",
      body: READ_TEXT,
      confirm_other_counterparty: false,
    });
  });

  it("files the reader's own words once they change the copied text", async () => {
    const sent = stubServer(DONE);
    show();
    const user = await pickScreenshot();

    const text = await screen.findByLabelText(en["chatCapture.text"]);
    await user.clear(text);
    await user.type(text, "Lena asked about Thursday.");

    expect(screen.queryByText(en["chatCapture.readBadge"])).toBeNull();
    await user.click(await fileButton());
    await waitFor(() => expect(filings(sent)).toHaveLength(1));
    expect(filedBody(sent)).toBe("Lena asked about Thursday.");
  });

  it("says why there is no text and still files the screenshot", async () => {
    const sent = stubServer(UNREAD);
    show();
    const user = await pickScreenshot();

    expect(await screen.findByText(UNREAD.status_detail ?? "")).toBeTruthy();
    expect(screen.getByText(en["chatCapture.unread"])).toBeTruthy();
    const text = screen.getByLabelText(en["chatCapture.text"]);
    expect((text as HTMLTextAreaElement).value).toBe("");

    await user.click(await fileButton());
    await waitFor(() => expect(filings(sent)).toHaveLength(1));
    expect(filedBody(sent)).toBe("");
  });

  it("archives the stored screenshot on Discard and files nothing", async () => {
    const sent = stubServer(DONE);
    const onClose = show();
    const user = await pickScreenshot();
    await screen.findByLabelText(en["chatCapture.text"]);

    await user.click(
      screen.getByRole("button", { name: en["chatCapture.discard"] }),
    );

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(
      sent.some(
        (s) => s.method === "DELETE" && s.path === "/v1/attachments/att-1",
      ),
    ).toBe(true);
    expect(filings(sent)).toHaveLength(0);
  });

  it("asks once before filing a chat that shows someone else", async () => {
    const sent = stubServer({ ...DONE, other_counterparty: "Jonas Reiter" });
    show();
    const user = await pickScreenshot();

    expect(
      await screen.findByText(
        "This chat shows Jonas Reiter, not Lena Hoffmann.",
      ),
    ).toBeTruthy();
    const file = await fileButton();
    expect((file as HTMLButtonElement).disabled).toBe(true);

    await user.click(screen.getByLabelText("File it on Lena Hoffmann anyway"));
    await user.click(file);

    await waitFor(() => expect(filings(sent)).toHaveLength(1));
    expect(
      (filings(sent)[0]?.body as { confirm_other_counterparty: boolean })
        .confirm_other_counterparty,
    ).toBe(true);
  });
});
