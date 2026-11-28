// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { ContractForm } from "./contractform";

// A term is refused on its own field, before sending and after.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubApi(answer: () => Response) {
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method === "POST") {
        writes.push(await request.json());
        return answer();
      }
      if (new URL(request.url).pathname.endsWith("/installation/settings")) {
        return Response.json({ base_currency: "EUR" });
      }
      return Response.json({ data: [] });
    }),
  );
  return writes;
}

function show(locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <ContractForm companyId="o-1" open onClose={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const COPY = {
  en: { title: /^Title/, notice: /^Notice period/, save: "Record contract" },
  de: { title: /^Titel/, notice: /^Kündigungsfrist/, save: "Vertrag erfassen" },
  vi: {
    title: /^Tiêu đề/,
    notice: /^Thời hạn báo trước/,
    save: "Ghi nhận hợp đồng",
  },
} as const;

it.each([
  ["en", "-1", "Notice period (days) must be at least 0."],
  ["de", "-1", "Kündigungsfrist (Tage) muss mindestens 0 sein."],
  ["vi", "-1", "Thời hạn báo trước (ngày) phải từ 0 trở lên."],
  ["en", "1.5", "Enter a whole number for Notice period (days)."],
] as const)(
  "refuses a notice period on its field before sending (%s, %s days)",
  async (locale, typed, sentence) => {
    const writes = stubApi(() => Response.json({ id: "c-1" }, { status: 201 }));
    show(locale);
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText(COPY[locale].title), "MSA");
    const notice = screen.getByLabelText(COPY[locale].notice);
    await user.type(notice, typed);

    expect(await screen.findByText(sentence)).toBeTruthy();
    expect(notice.getAttribute("aria-invalid")).toBe("true");
    await user.click(screen.getByRole("button", { name: COPY[locale].save }));
    await waitFor(() => expect(document.activeElement).toBe(notice));
    expect(writes).toEqual([]);
  },
);

it("puts the server's refusal on the term it names, by its label", async () => {
  const writes = stubApi(() =>
    Response.json(
      {
        code: "validation_error",
        detail: "`notice_period_days` must be an integer",
        details: {
          errors: [
            {
              field: "notice_period_days",
              code: "out_of_range",
              message: "`notice_period_days` must be at most 3650",
              params: { max: "3650" },
            },
          ],
        },
      },
      { status: 422 },
    ),
  );
  show();
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText(/^Title/), "MSA");
  const notice = screen.getByLabelText(/^Notice period/);
  await user.type(notice, "9000");
  await user.click(
    screen.getByRole("button", { name: englishCopy["contracts.form.save"] }),
  );

  const sentence = "Notice period (days) must be at most 3,650.";
  expect(await screen.findByText(sentence)).toBeTruthy();
  expect(writes).toHaveLength(1);
  expect(screen.queryByText(/notice_period_days/)).toBeNull();

  await user.type(notice, "0");
  expect(screen.queryByText(sentence)).toBeNull();
});
