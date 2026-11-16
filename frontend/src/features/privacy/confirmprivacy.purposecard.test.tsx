/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { vi as VIETNAMESE } from "../../i18n/vi";
import { ConfirmDetailsScreen } from "./confirm";

// The Vietnamese privacy page names a purpose and its basis for every reader.
// The basis is the consent with its date, or the agreement with the controller
// under Art. 19.1(d). For a contact with no use on record, the server gives
// the business correspondence purpose on the agreement.

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

const NOTICE = {
  kind: "privacy_notice",
  language: "vi",
  acquired_as: "public_or_business_source",
  rights: ["access", "objection"],
  data_categories: [],
  recipients: ["colleagues"],
  retention: [],
  controller: { legal_name: "Công ty Ví Dụ" },
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the Vietnamese privacy page", () => {
  it("names the purpose of a contact with no use on record, on the agreement", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        ...NOTICE,
        purposes: ["Business correspondence"],
        purpose_keys: ["business_correspondence"],
        purpose_bases: ["contract"],
        purpose_since: [null],
      }),
    );

    render(<ConfirmDetailsScreen token="tok-vi-notice" />);

    expect(
      await screen.findByText(VIETNAMESE["privacynotice.purposes.title"]),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /thực hiện thỏa thuận giữa quý khách và Công ty Ví Dụ theo Điều 19\.1\(d\) Luật Bảo vệ dữ liệu cá nhân/,
      ),
    ).toBeInTheDocument();
  });

  it("dates a consent the purpose rests on", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        ...NOTICE,
        purposes: ["Marketing email"],
        purpose_keys: ["marketing_email"],
        purpose_bases: ["consent"],
        purpose_since: ["2026-03-02T09:00:00Z"],
      }),
    );

    render(<ConfirmDetailsScreen token="tok-vi-consent" />);

    expect(
      await screen.findByText(/sự đồng ý của quý khách ngày/),
    ).toBeInTheDocument();
  });

  it("states the objection right as Vietnamese law gives it", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        ...NOTICE,
        purposes: ["Business correspondence"],
        purpose_keys: ["business_correspondence"],
        purpose_bases: ["contract"],
        purpose_since: [null],
      }),
    );

    render(<ConfirmDetailsScreen token="tok-vi-objection" />);

    expect(await screen.findByText("Quyền phản đối")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Quý khách có thể phản đối việc xử lý dữ liệu cá nhân hoặc rút lại sự đồng ý bất cứ lúc nào. Chúng tôi phản hồi trong 02 ngày làm việc và ngừng xử lý trong 15 ngày, trừ trường hợp pháp luật cho phép xử lý không cần sự đồng ý của quý khách. Nếu quý khách từ chối nhận quảng cáo, chúng tôi ngừng gửi quảng cáo ngay mà không cần quý khách nêu lý do.",
      ),
    ).toBeInTheDocument();
  });
});

describe("a Vietnamese reader with nothing on record", () => {
  it("is told no basis is recorded, and no agreement is claimed", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        ...NOTICE,
        purposes: ["Business correspondence"],
        purpose_keys: ["business_correspondence"],
        purpose_bases: ["none"],
        purpose_since: [null],
      }),
    );

    render(<ConfirmDetailsScreen token="tok-vi-none" />);

    expect(
      await screen.findByText(/chưa có căn cứ xử lý được ghi nhận/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Điều 19\.1\(d\)/)).not.toBeInTheDocument();
  });
});
