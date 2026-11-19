// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { looksLikeDataQuestion, looksLikeQuestion } from "./question";

describe("looksLikeQuestion", () => {
  it.each([
    "Wie stelle ich meine Zeitzone ein?",
    "how do I change my time zone",
    "where do I set the reporting timezone",
    "Làm sao để đổi múi giờ?",
    "đổi múi giờ như thế nào",
    "my password stopped working?",
    "Wo finde ich die Einstellungen？",
    "can I export my contacts",
    "kann man Rollen ändern",
    "cách nào để đổi múi giờ",
    "tháng này tôi thắng bao nhiêu deal",
    "thang nay toi thang bao nhieu deal",
    "Cuối tháng có đạt mục tiêu không",
    "tháng này tôi thắng mấy deal",
    "deal lớn nhất là của ai",
    "ai đã thắng deal Acme",
    "deal Acme đóng khi nào",
    "deal Acme là gì",
    "Mấy deal tôi thắng tháng này",
    "who owns the Acme deal",
    "wer betreut den Acme Deal",
  ])("reads %j as a question", (query) => {
    expect(looksLikeQuestion(query)).toBe(true);
  });

  it.each([
    "",
    "Zeitzone",
    "how?",
    "Acme GmbH",
    "Wo Holdings",
    "Anna Schmidt Berlin",
    "pipeline deals open",
    "Lam Van Minh",
    "Tai Bao Khi",
    "Can Do Logistics",
    "Is Kim Holdings",
    "Công ty Máy Việt",
    "Ai Holdings Vietnam",
    "Bao Nhi Minh",
    "Nguyễn Văn Mấy",
    "Trần Thị Nào",
    "Công ty TNHH Mấy",
    "Khách sạn La Gi Resort",
    "khách sạn la gi resort",
    "Cho Ai Market",
    "Bao Gio Nguyen",
    "Khi Nao Pham",
    "Cty Nào Đó",
    "cty nào đó Hà Nội",
    "Who Gives A Crap",
    "Wer Bau GmbH",
    "danh sách deal thắng của khách hàng mấy tháng gần đây",
    "  ",
  ])("reads %j as a lookup", (query) => {
    expect(looksLikeQuestion(query)).toBe(false);
  });
});

describe("looksLikeDataQuestion", () => {
  it.each([
    "How many deals did I win this month?",
    "wie viele Deals habe ich gewonnen",
    "tháng này tôi thắng bao nhiêu deal",
    "thang nay toi thang bao nhieu deal",
    "Cuối tháng có đạt mục tiêu không?",
    "tháng này tôi thắng mấy deal",
    "tổng doanh thu quý này",
    "did we reach the target this month",
    "haben wir das Ziel erreicht",
  ])("sends %j to the data reader", (question) => {
    expect(looksLikeDataQuestion(question)).toBe(true);
  });

  it.each([
    "how do I change my time zone",
    "Làm sao để đổi múi giờ?",
    "Tống Văn Minh là ai",
    "máy in ở đâu",
    "wonderful onboarding steps?",
    "how do I set a sales target",
    "where is the quota setting",
    "wie lege ich ein Ziel fest",
    "wie ändere ich den Umsatz Bericht",
    "anh Thắng làm ở công ty nào",
    "làm sao để đặt mục tiêu bán hàng",
    "danh sách khách hàng mấy tháng gần đây",
  ])("sends %j to the handbook", (question) => {
    expect(looksLikeDataQuestion(question)).toBe(false);
  });
});
