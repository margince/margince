// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { ArchivePurposeCard } from "./archivepurpose";
import { ControllerParticularsCard } from "./controllerparticulars";

const STORED = {
  legal_name: "Example Trading GmbH",
  postal_address: "Hauptstraße 1",
  privacy_contact: "privacy@example.com",
  privacy_page: "https://example.com/privacy",
  objection_route: "Write to privacy@example.com.",
  advertiser_contact: "",
  archive_purpose: "keep business records",
  // A field a newer server stores and neither screen knows.
  signature_line: "Example Trading GmbH, Berlin",
  translations: [
    {
      language: "vi",
      objection_route: "gửi email",
      signature_line: "Example Trading GmbH, Berlin",
    },
  ],
  disclosure_languages: ["vi"],
  objection_route_means: [],
};

const reply = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(card: "purpose" | "particulars") {
  const puts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/v1/me")) {
        return reply(
          meFixture({ allow: { installation_settings: ["read", "update"] } }),
        );
      }
      if (url.pathname.endsWith("/privacy/controller-particulars")) {
        if (request.method === "PUT") {
          const body = await request.json();
          puts.push(body);
          return reply({ ...STORED, ...body });
        }
        return reply(STORED);
      }
      throw new Error(`Unexpected request: ${request.method} ${url.pathname}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SavePageProvider>
          {card === "purpose" ? (
            <ArchivePurposeCard />
          ) : (
            <ControllerParticularsCard />
          )}
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { puts };
}

async function ready(name: string) {
  return waitFor(() => {
    const box = screen.getByRole("textbox", { name });
    if (!(box instanceof HTMLTextAreaElement) || box.disabled) {
      throw new Error(`${name} is not ready`);
    }
    return box;
  });
}

it("saves the Vietnamese purpose and sends every other particular back unchanged", async () => {
  const user = userEvent.setup();
  const { puts } = show("purpose");
  const vietnamese = await ready("Purpose of the archive in Vietnamese");
  expect(
    screen.getByRole("textbox", { name: "Purpose of the archive" }),
  ).toHaveValue("keep business records");
  await user.type(vietnamese, "lưu giữ hồ sơ kinh doanh");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]).toEqual({
    legal_name: "Example Trading GmbH",
    postal_address: "Hauptstraße 1",
    privacy_contact: "privacy@example.com",
    privacy_page: "https://example.com/privacy",
    objection_route: "Write to privacy@example.com.",
    advertiser_contact: "",
    archive_purpose: "keep business records",
    signature_line: "Example Trading GmbH, Berlin",
    translations: [
      {
        language: "vi",
        objection_route: "gửi email",
        signature_line: "Example Trading GmbH, Berlin",
        archive_purpose: "lưu giữ hồ sơ kinh doanh",
      },
    ],
  });
});

it("names the default purpose for an empty field", async () => {
  show("purpose");
  await ready("Purpose of the archive");
  expect(
    screen.getAllByText(
      /Empty, the page says “answer legal claims and keep business records”/,
    ),
  ).not.toHaveLength(0);
});

it("keeps the archive purpose, the privacy page and a field it does not know when the company details are saved", async () => {
  const user = userEvent.setup();
  const { puts } = show("particulars");
  const name = await waitFor(() => {
    const box = screen.getAllByRole("textbox")[0];
    if (!box || (box instanceof HTMLTextAreaElement && box.disabled)) {
      throw new Error("the form is not ready");
    }
    return box;
  });
  await user.type(name, " AG");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]).toMatchObject({
    legal_name: "Example Trading GmbH AG",
    privacy_page: "https://example.com/privacy",
    archive_purpose: "keep business records",
    signature_line: "Example Trading GmbH, Berlin",
    translations: [
      expect.objectContaining({
        language: "vi",
        signature_line: "Example Trading GmbH, Berlin",
      }),
    ],
  });
});
