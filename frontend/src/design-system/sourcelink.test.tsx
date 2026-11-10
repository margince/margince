/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Mail } from "lucide-react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SourceLink } from "./sourcelink";

afterEach(cleanup);

const subject = "Reminder: Margince Sneak Preview on 17 September";

describe("SourceLink", () => {
  it("opens the source, with its glyph before the name", () => {
    const onOpen = vi.fn();
    render(
      <SourceLink icon={<Mail aria-hidden="true" />} onOpen={onOpen}>
        {subject}
      </SourceLink>,
    );
    const control = screen.getByRole("button", { name: subject });
    expect(control.firstElementChild?.tagName.toLowerCase()).toBe("svg");
    // A record's name is data of any length, so it may wrap.
    expect(control.classList).toContain("btn-valuelabel");
    fireEvent.click(control);
    expect(onOpen).toHaveBeenCalledOnce();
  });

  it("is plain text when there is nowhere to open it", () => {
    render(
      <SourceLink icon={<Mail aria-hidden="true" />}>{subject}</SourceLink>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText(subject).classList).toContain("source-link-name");
  });
});
