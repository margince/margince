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
import { ControllerParticularsCard } from "./controllerparticulars";

const STORED = {
  legal_name: "Example Trading GmbH",
  postal_address: "Hauptstraße 1\n10115 Berlin",
  privacy_contact: "privacy@example.com",
  objection_route: "Write to privacy@example.com.",
  advertiser_contact: "",
  translations: [],
  advertising_languages: ["vi"],
  objection_route_means: ["web", "email", "phone"],
};

const MISSING_PHONE = {
  code: "validation_error",
  message: "The request has invalid fields.",
  details: {
    errors: [
      {
        field: "translations.vi.objection_route",
        code: "invalid",
        message:
          "add a phone number: this text must name each of a web page or social-network link, an email address, a phone number",
      },
    ],
  },
};

const reply = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show() {
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
          puts.push(await request.json());
          return reply(MISSING_PHONE, 422);
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
          <ControllerParticularsCard />
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { puts };
}

it("offers a Vietnamese version with the example route a pack in force asks for", async () => {
  show();
  const section = await screen.findByRole("region", { name: "Vietnamese" });
  expect(section).toHaveTextContent(
    "Example: trả lời email này với từ STOP, gửi email đến privacy@example.com, gọi số +84 00 0000 0000",
  );
  expect(
    screen.getAllByText(
      /Name a web page or social network link, an email address and a phone number/,
    ),
  ).toHaveLength(1);
});

it("sends the Vietnamese route and shows the refusal under its own field", async () => {
  const user = userEvent.setup();
  const { puts } = show();
  const section = await screen.findByRole("region", { name: "Vietnamese" });
  const route = await waitFor(() => {
    const boxes = section.querySelectorAll("textarea");
    const box = boxes[3];
    if (!box || box.disabled) throw new Error("the route box is not ready");
    return box;
  });
  await user.type(route, "gửi email đến privacy@example.com");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]).toMatchObject({
    translations: [
      { language: "vi", objection_route: "gửi email đến privacy@example.com" },
    ],
  });
  expect(await screen.findByText(/Add a phone number/)).toBeInTheDocument();
});
