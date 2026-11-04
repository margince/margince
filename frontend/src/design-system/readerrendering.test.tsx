// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi as viCatalog } from "../i18n/vi";
import { offersReaderRendering, ReaderRendering } from "./readerrendering";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";
const ORIGINAL = "Wir haben über die Verlängerung gesprochen.";

function answer(status: number, body: unknown) {
  const fetch = vi.fn(
    async (_request: Request) =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

function draw(locale: Locale, language: "de" | "en" | "vi" | null) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <ReaderRendering activityId={ACTIVITY} language={language}>
          <p>{ORIGINAL}</p>
        </ReaderRendering>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const rendered = {
  activity_id: ACTIVITY,
  source_language: "de",
  target_language: "en",
  body: "We talked about the renewal.",
  unchanged: false,
  model_used: "stub-model",
  generated_at: "2026-11-04T09:00:00Z",
};

describe("ReaderRendering", () => {
  it("offers the press beside another language or an unknown one, never beside the reader's own", () => {
    expect(offersReaderRendering("de", "en")).toBe(true);
    expect(offersReaderRendering(null, "en")).toBe(true);
    expect(offersReaderRendering(undefined, "vi")).toBe(true);
    expect(offersReaderRendering("en", "en")).toBe(false);
    draw("de", "de");
    expect(screen.getByText(ORIGINAL)).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: de["readerRendering.show"] }),
    ).toBeNull();
  });

  it("shows the rendering marked as AI-written, and the original one press away", async () => {
    const fetch = answer(200, rendered);
    draw("en", "de");
    fireEvent.click(
      screen.getByRole("button", { name: en["readerRendering.show"] }),
    );
    expect(await screen.findByText(rendered.body)).toBeTruthy();
    expect(screen.queryByText(ORIGINAL)).toBeNull();
    expect(screen.getByText(en["readerRendering.disclaimer"])).toBeTruthy();
    expect(screen.getByText(en["trust.agentUnnamed"])).toBeTruthy();
    const request = fetch.mock.calls[0]?.[0];
    expect(await request?.json()).toEqual({ locale: "en" });

    fireEvent.click(
      screen.getByRole("button", { name: en["readerRendering.showOriginal"] }),
    );
    expect(screen.getByText(ORIGINAL)).toBeTruthy();
    expect(screen.queryByText(rendered.body)).toBeNull();
  });

  it("says a text is already in the reader's language and keeps the original", async () => {
    answer(200, {
      ...rendered,
      unchanged: true,
      body: ORIGINAL,
      source_language: null,
    });
    draw("en", null);
    fireEvent.click(
      screen.getByRole("button", { name: en["readerRendering.show"] }),
    );
    expect(
      await screen.findByText(en["readerRendering.alreadyYours"]),
    ).toBeTruthy();
    expect(screen.getByText(ORIGINAL)).toBeTruthy();
  });

  for (const [locale, catalog] of [
    ["en", en],
    ["de", de],
    ["vi", viCatalog],
  ] as const) {
    for (const [code, status, key] of [
      ["ai_not_configured", 409, "readerRendering.noModel"],
      ["too_long", 422, "readerRendering.tooLong"],
    ] as const) {
      it(`says why a press was refused with ${code}, in ${locale}`, async () => {
        answer(status, { code, status, detail: "the server's English detail" });
        draw(locale, null);
        fireEvent.click(
          screen.getByRole("button", { name: catalog["readerRendering.show"] }),
        );
        expect(await screen.findByText(catalog[key])).toBeTruthy();
        expect(screen.queryByText("the server's English detail")).toBeNull();
      });
    }
  }
});
