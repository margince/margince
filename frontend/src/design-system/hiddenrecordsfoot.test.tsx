// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { HiddenRecordsFoot } from "./surfacestate";

afterEach(cleanup);

function inEnglish(node: ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

const dana = { user_id: "u-dana", display_name: "Dana" };
const minh = { user_id: "u-minh", display_name: "Minh" };

describe("HiddenRecordsFoot", () => {
  it("draws nothing when the scope withheld nothing, or the server sent no answer", () => {
    const { container } = inEnglish(
      <>
        <HiddenRecordsFoot hidden={{ count: 0, holders: [] }} />
        <HiddenRecordsFoot hidden={undefined} />
      </>,
    );
    expect(container.innerHTML).toBe("");
  });

  it("says how many are withheld and names every holder to ask", () => {
    inEnglish(
      <HiddenRecordsFoot hidden={{ count: 3, holders: [dana, minh] }} />,
    );
    expect(
      screen.getByText("3 more you cannot see. Ask Dana or Minh."),
    ).toBeTruthy();
  });

  it("states the count alone when every holder has left", () => {
    inEnglish(<HiddenRecordsFoot hidden={{ count: 2, holders: [] }} />);
    expect(screen.getByText("2 more you cannot see.")).toBeTruthy();
  });
});
