// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { PluralTranslator } from "../i18n";
import { taskRow, waitingEmailRow } from "../testing/brief.fixtures";
import { aboutFirst, readyLine, rowsAbout } from "./agentready";
import { spokenText } from "./ai-activity-speak";

const mail = waitingEmailRow();
const task = taskRow("task-1", "Send the proposal");
const day = [task, mail];

// The key and its figures, so a test reads which sentence was chosen.
const plural: PluralTranslator = (key, count, values) =>
  `${key}:${count}:${Object.values(values ?? {}).join("/")}`;

describe("rowsAbout", () => {
  it("keeps the rows about the open record", () => {
    expect(rowsAbout(day, { screen: "contacts", id: "contact-sonya" })).toEqual(
      [mail],
    );
  });

  it("keeps the rows about the kind of record a list shows", () => {
    expect(rowsAbout(day, { screen: "contacts" })).toEqual([mail]);
    expect(rowsAbout(day, { screen: "deals" })).toEqual([]);
  });

  it("claims nothing on a page that is not about records", () => {
    expect(rowsAbout(day, { screen: "home" })).toEqual([]);
    expect(rowsAbout(day, { screen: "settings", id: "contact-sonya" })).toEqual(
      [],
    );
  });
});

describe("aboutFirst", () => {
  it("leads with the open page's rows and keeps every other row once", () => {
    expect(aboutFirst(day, [mail])).toEqual([mail, task]);
  });
});

describe("readyLine", () => {
  it("says nothing when nothing waits", () => {
    expect(readyLine([], [], plural, "en")).toBeNull();
  });

  it("counts the day off a page with rows of its own", () => {
    const line = readyLine(day, [], plural, "en");
    expect(line && spokenText(line)).toBe("agent.ready.count:2:2");
  });

  it("says how many are about the open page", () => {
    const line = readyLine(day, [mail], plural, "en");
    expect(line && spokenText(line)).toBe("agent.ready.about:1:1/2");
  });

  it("counts the day when every row is about the open page", () => {
    const line = readyLine(day, day, plural, "en");
    expect(line && spokenText(line)).toBe("agent.ready.count:2:2");
  });
});
