// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS } from "../design-system/debouncedsearch";
import { LocaleProvider } from "../i18n";
import { AddDocumentDialog } from "./adddocument";

// Adding a link is one request: the address, the record and what the user
// chose. The page's own title fills the Title field until the user types one.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ME = {
  user: { id: "u-1", email: "rep@example.com", name: "Demo Rep" },
  authorization: {
    seat_type: "full",
    objects: { company: { update: true }, deal: { update: true } },
  },
};

type Sent = { url: string; body: unknown };

function stubApi(
  options: { preview?: Record<string, unknown>; addStatus?: number } = {},
) {
  const added: Sent[] = [];
  const previewed: string[] = [];
  const json = (payload: unknown, status = 200) =>
    new Response(JSON.stringify(payload), {
      status,
      headers: { "content-type": "application/json" },
    });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = request ? request.url : String(input);
      const body = request
        ? JSON.parse((await request.text()) || "null")
        : null;
      if (url.includes("/v1/me")) {
        return json(ME);
      }
      if (url.includes("/attachments/links/preview")) {
        previewed.push(body.url);
        return json({
          url: body.url,
          host: new URL(body.url).hostname,
          read: false,
          ...options.preview,
        });
      }
      if (url.includes("/attachments/links")) {
        added.push({ url, body });
        const status = options.addStatus ?? 201;
        return status === 201
          ? json(
              { id: "att-9", company_id: "o-1", filename: "acme.example" },
              201,
            )
          : json(
              {
                title: "Unprocessable",
                status,
                detail: "this address is already listed on the record",
              },
              status,
            );
      }
      return json({ data: [], page: { next_cursor: null } });
    }),
  );
  return { added, previewed };
}

function show(onClose = () => {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AddDocumentDialog
          anchor={{ record: "company", id: "o-1" }}
          open
          onClose={onClose}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The preview waits for the address to settle. Fake timers run that pause
// out at once, so no title check waits on the real clock.
async function chooseLink(user: UserEvent, address: string) {
  await user.click(screen.getByRole("button", { name: "Link" }));
  const field = screen.getByRole("textbox", { name: /Address/ });
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  try {
    fireEvent.change(field, { target: { value: address } });
    act(() => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
    });
  } finally {
    vi.useRealTimers();
  }
}

async function pressAdd(user: UserEvent) {
  const submit = screen.getByRole("button", { name: "Add link" });
  await waitFor(() => expect(submit.hasAttribute("disabled")).toBe(false));
  await user.click(submit);
}

describe("adding a link to a record's documents", () => {
  it("offers a link in place of the file", async () => {
    const user = userEvent.setup();
    stubApi();
    show();
    await user.click(screen.getByRole("button", { name: "Link" }));
    expect(screen.queryByText(/Drop a file here/)).toBeNull();
    expect(screen.getByRole("textbox", { name: /Address/ })).toBeTruthy();
  });

  it("fills the title the page gives, and sends one request", async () => {
    const user = userEvent.setup();
    const { added } = stubApi({ preview: { read: true, title: "Pricing" } });
    const onClose = vi.fn();
    show(onClose);
    await chooseLink(user, "https://acme.example/pricing");

    const title = screen.getByRole("textbox", { name: /Title/ });
    await waitFor(() =>
      expect((title as HTMLInputElement).value).toBe("Pricing"),
    );
    await pressAdd(user);

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(added).toHaveLength(1);
    expect(added[0]?.body).toEqual({
      entity_type: "company",
      entity_id: "o-1",
      url: "https://acme.example/pricing",
      title: "Pricing",
    });
  });

  it("keeps the title the user typed over the page's", async () => {
    const user = userEvent.setup();
    const { added } = stubApi({ preview: { read: true, title: "Pricing" } });
    show();
    await chooseLink(user, "https://acme.example/pricing");
    const title = screen.getByRole("textbox", { name: /Title/ });
    await waitFor(() =>
      expect((title as HTMLInputElement).value).toBe("Pricing"),
    );
    await user.clear(title);
    await user.type(title, "2027 price list");
    await pressAdd(user);
    await waitFor(() => expect(added).toHaveLength(1));
    expect(added[0]?.body).toMatchObject({ title: "2027 price list" });
  });

  it("asks for a title on a Google file, which Margince does not read", async () => {
    const user = userEvent.setup();
    stubApi();
    show();
    await chooseLink(user, "https://docs.google.com/document/d/abc/edit");
    expect(await screen.findByText(/does not read Google files/)).toBeTruthy();
  });

  it("refuses until the address is a web address", async () => {
    const user = userEvent.setup();
    const { added } = stubApi();
    show();
    await chooseLink(user, "acme.example");
    const submit = screen.getByRole("button", { name: "Add link" });
    await user.click(submit);
    expect(added).toHaveLength(0);
  });

  it("says what the server refused, and stays open", async () => {
    const user = userEvent.setup();
    stubApi({ addStatus: 422 });
    const onClose = vi.fn();
    show(onClose);
    await chooseLink(user, "https://acme.example/brief");
    await pressAdd(user);
    expect(await screen.findByText("Link not added")).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });
});
