// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type MailPreview = components["schemas"]["ControllerMailPreview"];
type MailLanguage = components["schemas"]["MailLanguage"];

// The server's own copy for both mails, with the link marker it puts where the
// link goes, so a test reads the words a contact would.
const COPY: Record<
  MailPreview["kind"],
  Record<MailLanguage, { subject: string; text: string }>
> = {
  record_confirmation: {
    en: {
      subject: "Review your contact details",
      text: "You can review the contact details we hold about you and correct any mistakes.\nChoosing to receive news from us is optional and separate from checking your details.\n\n[link]",
    },
    de: {
      subject: "Ihre Kontaktdaten prüfen",
      text: "Sie können Ihre bei uns gespeicherten Kontaktdaten prüfen und Fehler korrigieren.\nOb Sie außerdem Neuigkeiten von uns erhalten möchten, entscheiden Sie unabhängig davon.\n\n[Link]",
    },
    vi: {
      subject: "Kiểm tra thông tin liên hệ của quý khách",
      text: "Quý khách có thể kiểm tra thông tin liên hệ chúng tôi lưu và sửa thông tin chưa đúng.\nViệc đăng ký nhận tin là tùy chọn và độc lập với việc kiểm tra thông tin.\n\n[liên kết]",
    },
  },
  privacy_notice: {
    en: {
      subject: "Information about your personal data",
      text: "We hold personal data about you. The link below explains how to review the details, where they came from, how they are used, and your privacy rights.\nYou can also contact us directly using the contact details in this message.\n\n[link]\n\nNo reply or confirmation is required.",
    },
    de: {
      subject: "Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
      text: "Wir haben personenbezogene Daten von Ihnen gespeichert. Über den folgenden Link erfahren Sie, welche Daten das sind, woher sie stammen, wofür wir sie verwenden und welche Rechte Sie haben.\nBei Fragen können Sie sich auch direkt über die Kontaktdaten in dieser E-Mail an uns wenden.\n\n[Link]\n\nSie müssen weder antworten noch etwas bestätigen.",
    },
    vi: {
      subject: "Thông tin về việc xử lý dữ liệu cá nhân của quý khách",
      text: "Chúng tôi đang lưu dữ liệu cá nhân của quý khách. Liên kết dưới đây cho biết dữ liệu nào được lưu, nguồn cung cấp, cách sử dụng và các quyền về dữ liệu của quý khách.\nQuý khách cũng có thể liên hệ trực tiếp theo thông tin trong email này.\n\n[liên kết]\n\nQuý khách không cần trả lời hay xác nhận.",
    },
  },
};

export const RELAY_FROM = "privacy@kps.test";

/** A preview as the route answers it, for an installation that mails in German. */
export function mailPreview(
  kind: MailPreview["kind"],
  language: MailLanguage = "de",
  overrides: Partial<MailPreview> = {},
): MailPreview {
  return {
    kind,
    language,
    default_language: "de",
    languages: ["de", "en", "vi"],
    from_address: RELAY_FROM,
    to_address: "lena@kps.test",
    ...COPY[kind][language],
    ...overrides,
  };
}

/** What the route answers for a contact with no live address. */
export const NO_ADDRESS_PROBLEM = {
  title: "Unprocessable Entity",
  detail:
    "this contact carries no live email address, so there is no mailbox a mail could reach",
  status: 422,
};

/** What the route answers on an installation that cannot send its own mail. */
export const NO_MAIL_LANE_PROBLEM = {
  title: "Conflict",
  detail:
    "this installation cannot send its own mail, so the mail cannot be sent",
  status: 409,
};
