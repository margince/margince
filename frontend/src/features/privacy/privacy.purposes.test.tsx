// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { ConsentPurposesCard } from "./privacy.purposes";

// Add purpose checks what it can before it sends, and puts each refusal on the
// field it names, in the reader's words.

const TRANSACTIONAL = {
  id: "p1",
  key: "transactional",
  label: "Transactional email",
  requires_double_opt_in: false,
  class: "transactional",
  frequency: "monthly",
  created_at: "2026-01-02T09:00:00Z",
};

const NEWSLETTER = {
  ...TRANSACTIONAL,
  id: "p2",
  key: "ban_tin",
  label: "Bản tin",
  class: "marketing",
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stub(
  answer: () => Response,
  listed: unknown[] = [TRANSACTIONAL],
): unknown[] {
  const posted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (path.endsWith("/me")) {
        return json(
          meFixture({
            roles: ["admin"],
            allow: { consent_config: ["create"], contact: ["read"] },
          }),
        );
      }
      if (request.method === "POST" && path.endsWith("/consent-purposes")) {
        posted.push(await request.json());
        return answer();
      }
      if (path.endsWith("/consent-purposes")) {
        return json({ data: listed });
      }
      return json({ data: [], page: { next_cursor: null, has_more: false } });
    }),
  );
  return posted;
}

async function openAdd() {
  rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConsentPurposesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await screen.findByText(TRANSACTIONAL.label);
  await user.click(
    await screen.findByRole("button", { name: en["privacy.addPurpose"] }),
  );
  const dialog = await screen.findByRole("dialog");
  const keyBox = within(dialog).getByRole("textbox", {
    name: en["privacy.purposeKey"],
  });
  return { user, dialog, keyBox };
}

async function addPurpose(key: string, label: string) {
  const { user, dialog, keyBox } = await openAdd();
  if (key) await user.type(keyBox, key);
  if (label) {
    await user.type(
      within(dialog).getByRole("textbox", { name: en["privacy.purposeLabel"] }),
      label,
    );
  }
  await user.click(
    within(dialog).getByRole("button", { name: en["privacy.purposeCreate"] }),
  );
  return { user, dialog, keyBox };
}

const refused = (body: unknown, status: number) => () => json(body, status);

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Add purpose", () => {
  it("names the purpose that already holds the key, before sending", async () => {
    const posted = stub(refused({}, 500));
    const { user, keyBox } = await addPurpose(" Transactional", "Receipts");

    expect(
      await screen.findByText(
        "This key is already used by Transactional email.",
      ),
    ).toBeVisible();
    expect(keyBox).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(keyBox).toHaveFocus());
    expect(posted).toHaveLength(0);

    await user.type(keyBox, "_receipts");
    expect(keyBox).not.toHaveAttribute("aria-invalid", "true");
  });

  it("closes through its Cancel, sending nothing", async () => {
    const posted = stub(refused({}, 500));
    const { user, dialog } = await addPurpose("", "");
    await user.click(
      within(dialog).getByRole("button", { name: en["create.cancel"] }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(posted).toHaveLength(0);
  });

  it("says which fields are still empty instead of a grey button", async () => {
    const posted = stub(refused({}, 500));
    await addPurpose("", "");

    expect(await screen.findByText("Key is required.")).toBeVisible();
    expect(screen.getByText("Label is required.")).toBeVisible();
    expect(posted).toHaveLength(0);
  });

  it("puts a taken key the list did not hold on the Key field", async () => {
    stub(
      refused(
        {
          title: "Conflict",
          status: 409,
          code: "conflict",
          detail: 'purpose "retired": conflict',
        },
        409,
      ),
    );
    const { keyBox } = await addPurpose("retired", "Retired newsletter");

    expect(
      await screen.findAllByText(en["privacy.purposeKeyTakenUnnamed"]),
    ).toHaveLength(1);
    expect(keyBox).toHaveAttribute("aria-invalid", "true");
    expect(screen.queryByText(/conflict/)).toBeNull();
  });

  it("fills the key from the label until the admin types one", async () => {
    const posted = stub(() => json(TRANSACTIONAL, 201));
    const { user, dialog, keyBox } = await openAdd();
    const labelBox = within(dialog).getByRole("textbox", {
      name: en["privacy.purposeLabel"],
    });

    await user.type(labelBox, "Partner-News (Ü)");
    expect(keyBox).toHaveValue("partner_news_u");

    await user.clear(keyBox);
    await user.type(keyBox, "digest");
    await user.type(labelBox, " weekly");
    expect(keyBox).toHaveValue("digest");
    expect(posted).toHaveLength(0);
  });

  // Mutation: drop the shape check and the key with a space is sent.
  it("refuses a key with a space before sending", async () => {
    const posted = stub(refused({}, 500));
    const { keyBox } = await addPurpose("marketing email", "Mail digest");

    expect(
      await screen.findByText(en["privacy.purposeKeyShape"]),
    ).toBeVisible();
    expect(keyBox).toHaveAttribute("aria-invalid", "true");
    expect(posted).toHaveLength(0);
  });

  // The slip this guards: key and label swapped, so the label is a listed key.
  it.each([
    ["a listed key", "transactional"],
    ["a listed label, case and spacing aside", " transactional  EMAIL"],
  ])("refuses a label that reads as %s", async (_, label) => {
    const posted = stub(refused({}, 500));
    const { dialog } = await addPurpose("receipts", label);

    expect(
      await screen.findByText(
        "This label reads like Transactional email. Choose another label to tell the two purposes apart.",
      ),
    ).toBeVisible();
    expect(
      within(dialog).getByRole("textbox", { name: en["privacy.purposeLabel"] }),
    ).toHaveAttribute("aria-invalid", "true");
    expect(posted).toHaveLength(0);
  });

  // One label typed as a base letter and a combining mark reads as the listed one.
  // Mutation: drop the NFC step and the decomposed label is sent.
  it("refuses a label that differs from a listed one only in how its accent is encoded", async () => {
    const posted = stub(refused({}, 500), [TRANSACTIONAL, NEWSLETTER]);
    await addPurpose("tin_moi", "Ba\u0309n tin");

    expect(
      await screen.findByText(
        `This label reads like ${NEWSLETTER.label}. Choose another label to tell the two purposes apart.`,
      ),
    ).toBeVisible();
    expect(posted).toHaveLength(0);
  });

  // Case folds as on the server, so a capital sharp s reads as the listed one.
  // Mutation: lower-case without upper-casing first and STRASSE is sent.
  it("refuses a label that differs from a listed one only in a folded case", async () => {
    const listed = { ...NEWSLETTER, id: "p4", key: "anschrift", label: "Straße" };
    const posted = stub(refused({}, 500), [TRANSACTIONAL, listed]);
    await addPurpose("street", "STRASSE");

    expect(
      await screen.findByText(
        "This label reads like Straße. Choose another label to tell the two purposes apart.",
      ),
    ).toBeVisible();
    expect(posted).toHaveLength(0);
  });

  it("sends a label whose accent is another one", async () => {
    const created = {
      ...NEWSLETTER,
      id: "p3",
      key: "ban_tin_2",
      label: "Bán tin",
    };
    const posted = stub(() => json(created, 201), [TRANSACTIONAL, NEWSLETTER]);
    await addPurpose(created.key, created.label);

    await waitFor(() => expect(posted).toHaveLength(1));
  });

  it("puts a refused field on the field the refusal names, by its label", async () => {
    stub(
      refused(
        {
          title: "Unprocessable Entity",
          status: 422,
          code: "validation_error",
          details: {
            errors: [
              {
                field: "frequency",
                code: "invalid_enum",
                message: "pick weekly, monthly or quarterly",
              },
            ],
          },
        },
        422,
      ),
    );
    const { dialog } = await addPurpose("digest", "Weekly digest");

    const frequency = within(dialog).getByRole("combobox", {
      name: en["privacy.purposeFrequency"],
    });
    await waitFor(() =>
      expect(frequency).toHaveAttribute("aria-invalid", "true"),
    );
    expect(screen.queryByText(/pick weekly/)).toBeNull();
  });
});

function stubCounts(
  allow: NonNullable<Parameters<typeof meFixture>[0]>["allow"],
): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname.endsWith("/me")) {
        return json(meFixture({ roles: ["admin"], allow }));
      }
      if (url.pathname.endsWith("/consent-purposes/send-counts")) {
        asked.push(url.search);
        return json({
          days: 30,
          data: [
            {
              purpose_key: "transactional",
              class: "transactional",
              sends: 1200,
              disputed: 3,
            },
            { purpose_key: "unknown", sends: 2, disputed: 0 },
          ],
        });
      }
      return json({ data: [TRANSACTIONAL] });
    }),
  );
  return asked;
}

function renderCard() {
  rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ConsentPurposesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("Registered purposes", () => {
  it("shows each purpose's key under its label", async () => {
    stubCounts({ contact: ["read"] });
    renderCard();

    const table = await screen.findByRole("table", {
      name: en["privacy.purposesRegistry"],
    });
    const cell = within(table).getByText(TRANSACTIONAL.label).parentElement;
    expect(cell).toHaveTextContent(
      `${TRANSACTIONAL.label}${TRANSACTIONAL.key}`,
    );
  });
});

describe("Mail sent per purpose", () => {
  it("shows a compliance reader each purpose with its class and count", async () => {
    const asked = stubCounts({ consent_config: ["read"], contact: ["read"] });
    renderCard();

    const table = await screen.findByRole("table", {
      name: en["privacy.purposeSendsTable"],
    });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.map((row) => row.textContent)).toEqual([
      `${TRANSACTIONAL.label}${en["privacy.purposeClass.transactional"]}1,2003`,
      `${en["privacy.purposeUnknown"]}${en["privacy.purposeClass.undefined"]}20`,
    ]);
    expect(asked).toEqual(["?days=30"]);
  });

  it("asks nothing for a seat without the compliance read grant", async () => {
    const asked = stubCounts({ contact: ["read"] });
    renderCard();

    await screen.findByText(TRANSACTIONAL.label);
    expect(
      screen.queryByRole("table", { name: en["privacy.purposeSendsTable"] }),
    ).toBeNull();
    expect(asked).toEqual([]);
  });
});
