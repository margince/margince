/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ConfirmDetailsScreen } from "./confirm";

// The privacy page's storage line names why the archive keeps the reader's
// correspondence. It prints the purpose the installation stated in the link's
// language, or else the default purpose in that language.

const NOTICE = {
  kind: "privacy_notice",
  acquired_as: "unknown_legacy",
  purpose_bases: [],
  data_categories: [],
  recipients: ["colleagues"],
  retention: [{ scope: "activity", retain_days: 1095, action: "archive" }],
  rights: ["access"],
};

function show(page: unknown) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify(page), {
      headers: { "Content-Type": "application/json" },
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConfirmDetailsScreen token="tok-archive" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the archive line names the archive's purpose", () => {
  it.each([
    [
      "en",
      "answer claims under our supply contracts",
      "then moved to an archive with restricted access to answer claims under our supply contracts. They are deleted when they are no longer needed for this purpose",
    ],
    [
      "de",
      "Ansprüche aus unseren Lieferverträgen zu beantworten",
      "in ein Archiv mit eingeschränktem Zugriff verschoben, um Ansprüche aus unseren Lieferverträgen zu beantworten. Gelöscht werden sie, wenn sie für diesen Zweck nicht mehr nötig sind",
    ],
    [
      "vi",
      "giải quyết khiếu nại theo hợp đồng cung cấp",
      "chuyển vào kho lưu trữ hạn chế truy cập để giải quyết khiếu nại theo hợp đồng cung cấp và bị xóa khi không còn cần cho mục đích này",
    ],
  ])("in %s, the stated purpose", async (language, purpose, line) => {
    show({
      ...NOTICE,
      language,
      controller: {
        privacy_contact: "privacy@example.test",
        archive_purpose: purpose,
      },
    });
    expect(await screen.findByText(new RegExp(line))).toBeInTheDocument();
  });

  it.each([
    [
      "en",
      "restricted access to answer legal claims and keep business records.",
    ],
    [
      "de",
      "eingeschränktem Zugriff verschoben, um auf Rechtsansprüche zu antworten und Geschäftsunterlagen aufzubewahren.",
    ],
    [
      "vi",
      "hạn chế truy cập để giải quyết khiếu nại pháp lý và lưu giữ hồ sơ kinh doanh và bị xóa",
    ],
  ])(
    "in %s, the default purpose when none is stated",
    async (language, line) => {
      show({ ...NOTICE, language });
      expect(await screen.findByText(new RegExp(line))).toBeInTheDocument();
    },
  );
});
