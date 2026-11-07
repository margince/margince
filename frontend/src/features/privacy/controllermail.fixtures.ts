// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type MailPreview = components["schemas"]["ControllerMailPreview"];
type MailLanguage = components["schemas"]["MailLanguage"];

type MailLines = Readonly<{
  subject: string;
  intro: string;
  action: string;
  closing: string;
}>;

// Copied from backend/internal/platform/mailcopy. `mailText` joins them in the
// server's template order, without the sender line, the expiry and disclosures.
const MARKER: Record<MailLanguage, string> = {
  en: "[Personal link, created when you send]",
  de: "[Persönlicher Link, wird beim Senden erstellt]",
  vi: "[Liên kết cá nhân, được tạo khi bạn gửi]",
};

const PERSONAL: Record<MailLanguage, string> = {
  en: "This link is for your use only.",
  de: "Dieser Link ist nur für Sie bestimmt.",
  vi: "Liên kết này chỉ dành cho quý khách.",
};

const LINES: Record<MailPreview["kind"], Record<MailLanguage, MailLines>> = {
  record_confirmation: {
    en: {
      subject: "Review your contact details",
      intro:
        "You can review the contact details we hold about you and correct any mistakes.\nChoosing to receive news from us is optional and separate from checking your details.",
      action: "Review contact details",
      closing:
        "Reviewing your details is optional. Leaving this email unanswered does not subscribe you to anything.",
    },
    de: {
      subject: "Ihre Kontaktdaten prüfen",
      intro:
        "Sie können Ihre bei uns gespeicherten Kontaktdaten prüfen und Fehler korrigieren.\nOb Sie außerdem Neuigkeiten von uns erhalten möchten, entscheiden Sie unabhängig davon.",
      action: "Kontaktdaten ansehen",
      closing:
        "Die Prüfung Ihrer Daten ist freiwillig. Wenn Sie nichts tun, abonnieren Sie keine Nachrichten.",
    },
    vi: {
      subject: "Kiểm tra thông tin liên hệ của quý khách",
      intro:
        "Quý khách có thể kiểm tra thông tin liên hệ chúng tôi lưu và sửa thông tin chưa đúng.\nViệc đăng ký nhận tin là tùy chọn và độc lập với việc kiểm tra thông tin.",
      action: "Xem thông tin liên hệ",
      closing:
        "Việc kiểm tra thông tin là tùy chọn. Không trả lời email này không đăng ký quý khách nhận bất kỳ nội dung nào.",
    },
  },
  privacy_notice: {
    en: {
      subject: "Information about your personal data",
      intro:
        "We hold personal data about you. The link below explains how to review the details, where they came from, how they are used, and your privacy rights.\nYou can also contact us directly using the contact details in this message.",
      action: "View privacy information",
      closing: "No reply or confirmation is required.",
    },
    de: {
      subject: "Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
      intro:
        "Wir haben personenbezogene Daten von Ihnen gespeichert. Über den folgenden Link erfahren Sie, welche Daten das sind, woher sie stammen, wofür wir sie verwenden und welche Rechte Sie haben.\nBei Fragen können Sie sich auch direkt über die Kontaktdaten in dieser E-Mail an uns wenden.",
      action: "Datenschutzinformationen ansehen",
      closing: "Sie müssen weder antworten noch etwas bestätigen.",
    },
    vi: {
      subject: "Thông tin về việc xử lý dữ liệu cá nhân của quý khách",
      intro:
        "Chúng tôi đang lưu dữ liệu cá nhân của quý khách. Liên kết dưới đây cho biết dữ liệu nào được lưu, nguồn cung cấp, cách sử dụng và các quyền về dữ liệu của quý khách.\nQuý khách cũng có thể liên hệ trực tiếp theo thông tin trong email này.",
      action: "Xem thông tin bảo vệ dữ liệu",
      closing: "Quý khách không cần trả lời hay xác nhận.",
    },
  },
};

function mailText(lines: MailLines, language: MailLanguage): string {
  return [
    lines.intro,
    `${lines.action}\n  ${MARKER[language]}`,
    PERSONAL[language],
    `${lines.closing}\n`,
  ].join("\n\n");
}

/** The marker the server puts where the link goes in a preview. */
export function previewMarker(language: MailLanguage): string {
  return MARKER[language];
}

export const RELAY_FROM = "privacy@kps.test";

/** A preview in the route's shape, for an installation that mails in German. */
export function mailPreview(
  kind: MailPreview["kind"],
  language: MailLanguage = "de",
  overrides: Partial<MailPreview> = {},
): MailPreview {
  const lines = LINES[kind][language];
  return {
    kind,
    language,
    default_language: "de",
    languages: ["de", "en", "vi"],
    from_address: RELAY_FROM,
    to_address: "lena@kps.test",
    subject: lines.subject,
    text: mailText(lines, language),
    ...overrides,
  };
}

const NO_ADDRESS =
  "this contact carries no live email address, so there is no mailbox a confirm link could reach";

/** The server's 422 for a contact with no live address. */
export const NO_ADDRESS_PROBLEM = {
  type: "https://errors.gradion.com/validation_error",
  title: "Unprocessable Entity",
  status: 422,
  code: "validation_error",
  detail: NO_ADDRESS,
  details: {
    errors: [{ field: "contact_id", code: "invalid", message: NO_ADDRESS }],
  },
};

/** The server's 409 on an installation that cannot send its own mail. */
export const NO_MAIL_LANE_PROBLEM = {
  type: "https://errors.gradion.com/conflict",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail:
    "conflict: this installation cannot send its own mail yet; configure the relay, public URL and key vault",
};
