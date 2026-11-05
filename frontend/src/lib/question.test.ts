// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { looksLikeQuestion } from "./question";

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
    "  ",
  ])("reads %j as a lookup", (query) => {
    expect(looksLikeQuestion(query)).toBe(false);
  });
});
