/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { CompanyDocumentsCard } from "./companydocuments";

// A wrong upload on a company is removed or corrected from the account's
// library, by the same menu the deal's Files area uses.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Attachment = components["schemas"]["Attachment"];

const UPLOAD: Attachment = {
  id: "f-1",
  entity_type: "company",
  entity_id: "o-1",
  filename: "scan.pdf",
  category: "other",
  doc_state: "current",
  source: "upload",
  captured_by: "human:u-1",
  created_at: "2026-08-01T09:00:00Z",
};

const MAIL_FILE: Attachment = {
  ...UPLOAD,
  id: "f-2",
  entity_type: "activity",
  entity_id: "act-1",
  filename: "from-mail.pdf",
  category: "email_attachment",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function stubServer(files: Attachment[], mayUpdate = true) {
  const writes: { method: string; path: string; body: unknown }[] = [];
  const gone = new Set<string>();
  const patched = new Map<string, Partial<Attachment>>();
  vi.stubGlobal("fetch", async (request: Request) => {
    const { pathname } = new URL(request.url);
    if (pathname.endsWith("/me")) {
      return json({
        user: { id: "u-1" },
        authorization: {
          seat_type: "full",
          objects: { company: { read: true, update: mayUpdate } },
        },
      });
    }
    if (pathname.endsWith("/installation/settings")) {
      return json({ max_upload_bytes: 26_214_400 });
    }
    if (request.method === "GET") {
      if (pathname.endsWith("/companies/o-1/documents") === false) {
        return json({ data: [], page: {} });
      }
      const data = files
        .filter((file) => !gone.has(file.id))
        .map((file) => ({ ...file, ...patched.get(file.id) }));
      return json({ data, page: { next_cursor: null } });
    }
    const text = await request.text();
    const body = text ? JSON.parse(text) : null;
    writes.push({ method: request.method, path: pathname, body });
    const id = pathname.split("/")[3];
    if (request.method === "DELETE") {
      gone.add(id);
      return new Response(null, { status: 204 });
    }
    patched.set(id, body);
    return json({ ...UPLOAD, ...body });
  });
  return writes;
}

function show(writable: boolean | undefined = true) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyDocumentsCard companyId="o-1" writable={writable} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("deletes an uploaded document after a confirm", async () => {
  const writes = stubServer([UPLOAD]);
  const user = userEvent.setup();
  show();

  await user.click(
    await screen.findByRole("button", { name: /Actions for scan/ }),
  );
  await user.click(screen.getByRole("button", { name: en["files.delete"] }));
  await user.click(
    within(await screen.findByRole("dialog")).getByRole("button", {
      name: en["files.delete"],
    }),
  );

  await waitFor(() =>
    expect(writes).toEqual([
      { method: "DELETE", path: "/v1/attachments/f-1", body: null },
    ]),
  );
  await waitFor(() => expect(screen.queryByText("scan.pdf")).toBeNull());
});

it("edits title, category and state and sends only what changed", async () => {
  const writes = stubServer([UPLOAD]);
  const user = userEvent.setup();
  show();

  await user.click(
    await screen.findByRole("button", { name: /Actions for scan/ }),
  );
  await user.click(screen.getByRole("button", { name: en["files.edit"] }));
  const dialog = await screen.findByRole("dialog");
  await user.type(within(dialog).getByRole("textbox"), "Signed scan");
  await user.click(within(dialog).getByRole("combobox", { name: "State" }));
  await user.click(
    await screen.findByRole("option", { name: en["docs.state.superseded"] }),
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["files.save"] }),
  );

  await waitFor(() =>
    expect(writes).toEqual([
      {
        method: "PATCH",
        path: "/v1/attachments/f-1/metadata",
        body: { title: "Signed scan", doc_state: "superseded" },
      },
    ]),
  );
});

it("offers no menu on a deal's upload listed in the library", async () => {
  stubServer([{ ...UPLOAD, entity_type: "deal", entity_id: "d-1" }]);
  show();

  await screen.findByText("scan.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});

it("offers no menu on a file that came with a message", async () => {
  stubServer([MAIL_FILE]);
  show();

  await screen.findByText("from-mail.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});

it("shows no verbs to a seat that cannot write the company", async () => {
  stubServer([UPLOAD], false);
  show(true);

  await screen.findByText("scan.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});

it("shows no verbs when the company row is not writable", async () => {
  stubServer([UPLOAD]);
  show(false);

  await screen.findByText("scan.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});
