/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { ConfirmDetailsScreen } from "./confirm";

// What the public confirm pages say under the statutory wording. That covers
// the controller's labels, the storage period and the rights. It also covers
// the news question and the source of a contact typed in by hand.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
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

const CONTROLLER = {
  legal_name: "Công ty Ví Dụ",
  identity: "Công ty Ví Dụ\n1 Lê Lợi, Quận 1",
  privacy_contact: "privacy@vidu.test",
};

const NOTICE = {
  kind: "privacy_notice",
  acquired_as: "unknown_legacy",
  purpose_bases: [],
  data_categories: [],
  recipients: ["colleagues"],
  retention: [{ scope: "activity", retain_days: 1095, action: "archive" }],
  rights: [
    "access",
    "rectification",
    "erasure",
    "restriction",
    "portability",
    "objection",
    "withdraw_consent",
    "complain_to_authority",
  ],
};

const RECORD = {
  kind: "record_confirmation",
  full_name: "Nguyễn Văn A",
  title: "",
  company: "",
  email: "a@vidu.test",
  phone: "",
  marketing_state: "unknown",
  provenance: [],
  controller: CONTROLLER,
};

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the Vietnamese privacy page uses the statutory wording", () => {
  it("names the controller, the source, the storage period and every right", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ ...NOTICE, controller: CONTROLLER, language: "vi" }),
    );

    render(<ConfirmDetailsScreen token="tok-vi-notice" />);

    expect(
      await screen.findByRole("heading", {
        name: "Bên kiểm soát dữ liệu cá nhân",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Liên hệ về bảo vệ dữ liệu cá nhân: privacy@vidu.test"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Thông báo này cho quý khách biết về việc xử lý dữ liệu cá nhân của quý khách. Quý khách không cần trả lời hay làm gì.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Chúng tôi không xác định được nguồn thu thập thông tin của quý khách.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Email, cuộc họp và cuộc gọi với quý khách: được lưu trong 3 năm kể từ khi ghi nhận, sau đó được chuyển vào kho lưu trữ hạn chế truy cập để giải quyết khiếu nại pháp lý và lưu giữ hồ sơ kinh doanh và bị xóa khi không còn cần cho mục đích này, khi quý khách yêu cầu hoặc khi hồ sơ của quý khách bị xóa.",
      ),
    ).toBeInTheDocument();

    const rights = screen.getByRole("heading", {
      name: "Quyền của quý khách đối với thông tin này",
    }).parentElement;
    if (!rights) throw new Error("the rights card has no body");
    expect(
      within(rights)
        .getAllByRole("listitem")
        .map((item) => item.textContent),
    ).toEqual([
      "Yêu cầu bản sao thông tin đang lưu về quý khách.",
      "Yêu cầu sửa những thông tin chưa đúng.",
      "Yêu cầu xóa thông tin.",
      "Yêu cầu hạn chế xử lý thông tin khi quý khách có nghi ngờ về phạm vi, mục đích xử lý hoặc tính chính xác của thông tin.",
      "Yêu cầu nhận dữ liệu quý khách đã cung cấp ở định dạng phổ biến, máy đọc được, hoặc chuyển dữ liệu đó cho bên khác.",
      "Rút lại sự đồng ý đã cho bất cứ lúc nào. Việc này không thay đổi những gì chúng tôi đã làm trước đó.",
      "Khiếu nại, tố cáo, khởi kiện và yêu cầu bồi thường thiệt hại theo quy định của pháp luật. Tại Việt Nam, cơ quan có thẩm quyền về bảo vệ dữ liệu cá nhân là cơ quan chuyên trách bảo vệ dữ liệu cá nhân thuộc Bộ Công an.",
      "Yêu cầu cơ quan có thẩm quyền hoặc tổ chức, cá nhân liên quan áp dụng biện pháp bảo vệ dữ liệu cá nhân của quý khách.",
    ]);
    expect(
      within(rights).getByText(
        "Để sử dụng bất kỳ quyền nào, vui lòng liên hệ: privacy@vidu.test. Chúng tôi phản hồi trong 02 ngày làm việc kể từ khi nhận được yêu cầu.",
      ),
    ).toBeInTheDocument();
  });
});

describe("a contact typed in by hand names its source", () => {
  it("prints the stated source and note instead of an unknown source", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        ...NOTICE,
        stated_source: "business_card",
        stated_source_note: "Trade fair in Hanoi",
      }),
    );

    render(<ConfirmDetailsScreen token="tok-stated" />);

    expect(
      await screen.findByText(
        englishCopy["privacynotice.source.stated.businessCard"],
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText("In our words: Trade fair in Hanoi"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(englishCopy["privacynotice.source.unknown"]),
    ).toBeNull();
  });

  it("says the source is unknown only when none was given", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(NOTICE));

    render(<ConfirmDetailsScreen token="tok-unstated" />);

    expect(
      await screen.findByText(englishCopy["privacynotice.source.unknown"]),
    ).toBeInTheDocument();
  });
});

describe("the record page's news question", () => {
  const ASKS = {
    en: {
      text: "Công ty Ví Dụ will use your name and email address to send you news about its products and services by email, roughly once a month. You can withdraw your consent at any time with the unsubscribe link in each email.",
      yes: "Subscribe to news",
      no: "Do not send news",
    },
    de: {
      text: "Công ty Ví Dụ verwendet Ihren Namen und Ihre E-Mail-Adresse, um Ihnen etwa einmal im Monat per E-Mail Neuigkeiten zu eigenen Produkten und Dienstleistungen zu senden. Sie können Ihre Einwilligung jederzeit über den Abmeldelink in jeder E-Mail widerrufen.",
      yes: "Neuigkeiten abonnieren",
      no: "Keine Neuigkeiten senden",
    },
    vi: {
      text: "Công ty Ví Dụ sẽ dùng họ tên và địa chỉ email của quý khách để gửi email tin tức về sản phẩm và dịch vụ của mình, khoảng mỗi tháng một lần. Quý khách có thể rút lại sự đồng ý bất cứ lúc nào bằng liên kết hủy đăng ký trong mỗi email.",
      yes: "Đăng ký nhận tin",
      no: "Không gửi tin cho tôi",
    },
  } as const;

  for (const [language, ask] of Object.entries(ASKS)) {
    it(`names the sender, the data and the way out, with two equal buttons, in ${language}`, async () => {
      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        jsonResponse({ ...RECORD, language, asks_news: true }),
      );

      render(<ConfirmDetailsScreen token={`tok-news-${language}`} />);

      expect(await screen.findByText(ask.text)).toBeInTheDocument();
      const yes = screen.getByRole("button", { name: ask.yes });
      const no = screen.getByRole("button", { name: ask.no });
      expect(no.tagName).toBe(yes.tagName);
      expect(no.className).toBe(yes.className);
    });
  }

  it("is left out of a record check whose recipient may not be asked", async () => {
    const user = userEvent.setup();
    const posted: Record<string, unknown>[] = [];
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const request = input instanceof Request ? input : undefined;
      if ((request?.method ?? init?.method ?? "GET") === "GET") {
        return jsonResponse({ ...RECORD, language: "de", asks_news: false });
      }
      posted.push(
        JSON.parse(request ? await request.text() : String(init?.body)),
      );
      return jsonResponse({ cases: [] });
    });

    render(<ConfirmDetailsScreen token="tok-de-check" />);

    const submit = await screen.findByRole("button", {
      name: "Angaben bestätigen",
    });
    expect(
      screen.queryByRole("heading", {
        name: "Gelegentlich Neuigkeiten erhalten?",
      }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Neuigkeiten abonnieren" }),
    ).toBeNull();
    await user.click(submit);
    await screen.findByRole("heading", { name: "Vielen Dank" });
    expect(posted).toHaveLength(1);
    expect(posted[0]).not.toHaveProperty("marketing_choice");
  });
});

describe("the privacy page states AI providers, transfers and automated decisions", () => {
  const LINES = {
    en: /^We use AI services to sort, summarize and draft text.*No decision that has legal effects for you/,
    de: /^Wir nutzen KI-Dienste.*Keine Entscheidung mit rechtlicher Wirkung/,
    vi: /^Chúng tôi dùng dịch vụ AI.*Không có quyết định nào gây hậu quả pháp lý/,
  };
  for (const [language, line] of Object.entries(LINES)) {
    it(`renders the line in ${language}`, async () => {
      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        jsonResponse({ ...NOTICE, language }),
      );

      render(<ConfirmDetailsScreen token={`tok-transfers-${language}`} />);

      expect(await screen.findByText(line)).toBeInTheDocument();
    });
  }
});
