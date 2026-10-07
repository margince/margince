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
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { DealFiles } from "./dealfiles";

// The deal's Files area as a rep meets it: a captured file says which message
// it came with and offers Hide, an upload offers Delete, and a hide lands on
// the deal's own hide route at once, with an Undo, rather than touching the file.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Deal = components["schemas"]["Deal"];
type DealDocument = components["schemas"]["DealDocument"];

// The deal the files hang off, as the server sends it to a caller who owns
// it: `writable` is what every write here is gated on, so the fixture states
// it rather than leaving the panel to fail closed on an absent flag.
function dealOf(overrides: Partial<Deal> = {}): Deal {
  return {
    id: "deal-1",
    name: "Fleet retrofit",
    pipeline_id: "pl",
    stage_id: "s1",
    status: "open",
    source: "manual",
    captured_by: "human:u1",
    writable: true,
    version: 1,
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    ...overrides,
  } as Deal;
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`), so a
            suite whose subject includes what a hide SAYS mounts it the same
            way — the Undo this screen offers lives inside it. */}
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
};

function upload(): DealDocument {
  return {
    hidden: false,
    attachment: {
      id: "att-up",
      entity_type: "deal",
      entity_id: "deal-1",
      filename: "pricing.pdf",
      category: "offer",
      source: "upload",
      captured_by: "human:u1",
      created_at: "2026-08-20T09:00:00Z",
    },
  } as DealDocument;
}

function captured(hidden = false): DealDocument {
  return {
    hidden,
    attachment: {
      id: "att-mail",
      entity_type: "activity",
      entity_id: "act-1",
      filename: "MSA-redline.docx",
      category: "email_attachment",
      source: "gmail",
      captured_by: "human:u1",
      created_at: "2026-08-21T09:00:00Z",
    },
    origin: {
      activity_id: "act-1",
      kind: "email",
      subject: "Re: MSA",
      occurred_at: "2026-08-21T08:55:00Z",
      counterparty_email: "laura@buyer.example",
    },
  } as DealDocument;
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function me() {
  return {
    user: { id: "u1" },
    authorization: {
      seat_type: "full",
      objects: {
        deal: { create: true, read: true, update: true, delete: true },
      },
    },
  };
}

/**
 * The backend: it records every write and keeps the hide flag, so a hidden row
 * leaves the default read. `refuse` answers a write with a problem document.
 */
function stubApi(
  docs: DealDocument[],
  refuse?: (request: Request) => boolean,
): { calls: Request[] } {
  const calls: Request[] = [];
  const hidden = new Set(
    docs.filter((doc) => doc.hidden).map((doc) => doc.attachment.id),
  );
  vi.stubGlobal("fetch", (input: Request) => {
    const url = new URL(input.url);
    if (input.method !== "GET") {
      calls.push(input.clone());
      if (refuse?.(input)) {
        return Promise.resolve(
          jsonResponse({ detail: "the message was deleted" }, 409),
        );
      }
      const hideOf = /\/documents\/([^/]+)\/hide$/.exec(url.pathname);
      if (hideOf && input.method === "PUT") {
        hidden.add(hideOf[1]);
      } else if (hideOf) {
        hidden.delete(hideOf[1]);
      }
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url.pathname.endsWith("/me")) {
      return Promise.resolve(jsonResponse(me()));
    }
    const withHidden = url.searchParams.get("include_hidden") === "true";
    const data = docs
      .map((doc) => ({ ...doc, hidden: hidden.has(doc.attachment.id) }))
      .filter((doc) => withHidden || !doc.hidden);
    return Promise.resolve(jsonResponse({ data, page: {} }));
  });
  return { calls };
}

async function hideFromMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: /Actions for MSA-redline/ }),
  );
  await user.click(screen.getByRole("button", { name: en["files.hide"] }));
}

it("tells a captured file from an upload and says where it came from", async () => {
  stubApi([upload(), captured()]);
  render(<DealFiles deal={dealOf()} />);

  expect(await screen.findByText("MSA-redline.docx")).toBeInTheDocument();
  expect(
    screen.getByText(/Attachment of a message from laura@buyer.example/),
  ).toBeInTheDocument();
  expect(screen.getByText(/Uploaded/)).toBeInTheDocument();
});

it("hides a captured file at once through the deal's own hide route, never the file", async () => {
  const { calls } = stubApi([captured()]);
  const user = userEvent.setup();
  render(<DealFiles deal={dealOf()} />);

  await hideFromMenu(user);

  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0].method).toBe("PUT");
  expect(new URL(calls[0].url).pathname).toBe(
    "/v1/deals/deal-1/documents/att-mail/hide",
  );
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("hands focus to the list the hidden row left, not to the page", async () => {
  stubApi([captured()]);
  const user = userEvent.setup();
  render(<DealFiles deal={dealOf()} />);

  await hideFromMenu(user);

  const empty = await screen.findByText(en["files.empty"]);
  await waitFor(() => {
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement).toContainElement(empty);
  });
});

it("offers Delete on an upload and no Hide", async () => {
  stubApi([upload()]);
  const user = userEvent.setup();
  render(<DealFiles deal={dealOf()} />);

  await user.click(
    await screen.findByRole("button", { name: /Actions for pricing/ }),
  );
  expect(screen.getByRole("button", { name: "Delete" })).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "Hide from this deal" }),
  ).not.toBeInTheDocument();
});

// The Undo is the hide's only way back, so it has to restore the row exactly
// and has to say so when it cannot.
it("puts a hidden file back through the Undo the confirmation carries", async () => {
  const { calls } = stubApi([captured()]);
  const user = userEvent.setup();
  render(<DealFiles deal={dealOf()} />);

  await hideFromMenu(user);

  const said = await screen.findByRole("status");
  expect(said).toHaveTextContent(en["dealfiles.hidden"]);
  await user.click(
    within(said).getByRole("button", { name: en["common.undo"] }),
  );

  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1].method).toBe("DELETE");
  expect(new URL(calls[1].url).pathname).toBe(
    "/v1/deals/deal-1/documents/att-mail/hide",
  );
  expect(await screen.findByRole("status")).toHaveTextContent(
    en["dealfiles.unhidden"],
  );
  expect(await screen.findByText("MSA-redline.docx")).toBeInTheDocument();
});

it("keeps a refused hide on screen as a danger toast, since no dialog is open to hold it", async () => {
  stubApi([captured()], (request) => request.method === "PUT");
  const user = userEvent.setup();
  render(<DealFiles deal={dealOf()} />);

  await hideFromMenu(user);

  const said = await screen.findByRole("status");
  await waitFor(() =>
    expect(said).toHaveTextContent("the message was deleted"),
  );
  expect(said.querySelector(".toast-dot-danger")).not.toBeNull();
  // Sticky: a self-withdrawing toast draws no dismiss control.
  expect(
    within(said).getByRole("button", { name: en["common.close"] }),
  ).toBeInTheDocument();
  expect(screen.getByText("MSA-redline.docx")).toBeInTheDocument();
});

it("says so when the Undo is refused, rather than letting it fail quietly", async () => {
  // The message the Undo was offered from is consumed by the press, so a
  // silent refusal leaves the reader watching a confirmation disappear and
  // believing the file came back.
  stubApi([captured()], (request) => request.method === "DELETE");
  const user = userEvent.setup();
  render(<DealFiles deal={dealOf()} />);

  await hideFromMenu(user);
  await user.click(
    within(await screen.findByRole("status")).getByRole("button", {
      name: en["common.undo"],
    }),
  );

  expect(
    await screen.findByText("the message was deleted"),
  ).toBeInTheDocument();
});

// `writable` is the server's per-row answer, and it is what the upload and
// every row verb are gated on: a rep holding deal.update on the OBJECT was
// offered Add file on a colleague's deal and refused once the bytes were
// chosen.
it("withholds the upload and the row verbs on a deal this caller may not write", async () => {
  stubApi([upload(), captured()]);
  render(<DealFiles deal={dealOf({ owner_id: "u-other", writable: false })} />);

  expect(await screen.findByText("MSA-redline.docx")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "Add document" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: /Actions for MSA-redline/ }),
  ).toBeNull();
});
