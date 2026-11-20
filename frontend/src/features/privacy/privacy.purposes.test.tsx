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

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stub(answer: () => Response): unknown[] {
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
        return json({ data: [TRANSACTIONAL] });
      }
      return json({ data: [], page: { next_cursor: null, has_more: false } });
    }),
  );
  return posted;
}

async function addPurpose(key: string, label: string) {
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
