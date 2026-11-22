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
import { ContactFilesTab } from "./contactfiles";

// A wrong upload on a contact is removed or corrected from the contact's own
// Documents tab, by the same menu the deal's Files area uses.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Attachment = components["schemas"]["Attachment"];

const UPLOAD: Attachment = {
  id: "f-1",
  entity_type: "contact",
  entity_id: "p-1",
  filename: "scan.pdf",
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

function stubServer(
  files: Attachment[],
  mayUpdate = true,
  refuseWritesWith?: number,
) {
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
          objects: { contact: { read: true, update: mayUpdate } },
        },
      });
    }
    if (pathname.endsWith("/installation/settings")) {
      return json({ max_upload_bytes: 26_214_400 });
    }
    if (request.method === "GET") {
      const data = files
        .filter((file) => !gone.has(file.id))
        .map((file) => ({ ...file, ...patched.get(file.id) }));
      return json({ data, page: { has_more: false, next_cursor: null } });
    }
    if (refuseWritesWith) {
      return json(
        { title: "Refused", detail: "Refused for the test" },
        refuseWritesWith,
      );
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
        <ContactFilesTab
          contactId="p-1"
          contactName="Ada"
          writable={writable}
        />
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

it("offers no menu on a file that came with a message", async () => {
  stubServer([MAIL_FILE]);
  show();

  await screen.findByText("from-mail.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});

it("shows no verbs to a seat that cannot write the contact", async () => {
  stubServer([UPLOAD], false);
  show(true);

  await screen.findByText("scan.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});

it("shows no verbs when the contact row is not writable", async () => {
  stubServer([UPLOAD]);
  show(false);

  await screen.findByText("scan.pdf");
  expect(screen.queryByRole("button", { name: /Actions for/ })).toBeNull();
});

async function openEdit(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: /Actions for/ }));
  await user.click(screen.getByRole("button", { name: en["files.edit"] }));
  return screen.findByRole("dialog");
}

it("sends a null title when the title is cleared", async () => {
  const writes = stubServer([{ ...UPLOAD, title: "Old name" }]);
  const user = userEvent.setup();
  show();

  const dialog = await openEdit(user);
  await user.clear(within(dialog).getByRole("textbox"));
  await user.click(
    within(dialog).getByRole("button", { name: en["files.save"] }),
  );

  await waitFor(() =>
    expect(writes.map((write) => write.body)).toEqual([{ title: null }]),
  );
});

it("offers Category on an upload with no category and sends only a change to it", async () => {
  const writes = stubServer([UPLOAD]);
  const user = userEvent.setup();
  show();

  const dialog = await openEdit(user);
  await user.click(
    within(dialog).getByRole("combobox", { name: en["docs.add.category"] }),
  );
  await user.click(
    await screen.findByRole("option", { name: en["docs.category.legal"] }),
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["files.save"] }),
  );

  await waitFor(() =>
    expect(writes.map((write) => write.body)).toEqual([{ category: "legal" }]),
  );
});

it("sends nothing when the dialog is saved unchanged", async () => {
  const writes = stubServer([UPLOAD]);
  const user = userEvent.setup();
  show();

  const dialog = await openEdit(user);
  await user.click(
    within(dialog).getByRole("button", { name: en["files.save"] }),
  );

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(writes).toEqual([]);
});

it("hides Category for a link, which has no assignable category", async () => {
  stubServer([{ ...UPLOAD, category: "link", url: "https://example.com/a" }]);
  const user = userEvent.setup();
  show();

  const dialog = await openEdit(user);
  expect(
    within(dialog).queryByRole("combobox", { name: en["docs.add.category"] }),
  ).toBeNull();
});

it.each([403, 404, 500])(
  "says so and keeps the row when an edit is refused with %i",
  async (status) => {
    stubServer([UPLOAD], true, status);
    const user = userEvent.setup();
    show();

    const dialog = await openEdit(user);
    await user.type(within(dialog).getByRole("textbox"), "New name");
    await user.click(
      within(dialog).getByRole("button", { name: en["files.save"] }),
    );

    expect(await within(dialog).findByRole("alert")).toBeInTheDocument();
    expect(screen.getByText("scan.pdf")).toBeInTheDocument();
  },
);

it.each([403, 404, 500])(
  "says so and keeps the row when a delete is refused with %i",
  async (status) => {
    stubServer([UPLOAD], true, status);
    const user = userEvent.setup();
    show();

    await user.click(
      await screen.findByRole("button", { name: /Actions for scan/ }),
    );
    await user.click(screen.getByRole("button", { name: en["files.delete"] }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: en["files.delete"] }),
    );

    expect(await within(dialog).findByRole("alert")).toBeInTheDocument();
    expect(screen.getByText("scan.pdf")).toBeInTheDocument();
  },
);
