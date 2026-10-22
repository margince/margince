// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PageHeadAction } from "./pageheadaction";

// Under the shell, the verb's place is shell.pagehead.test.tsx's.

afterEach(cleanup);

describe("PageHeadAction with no shell", () => {
  it("draws the verb where it is written", () => {
    render(
      <PageHeadAction>
        <button type="button">New team</button>
      </PageHeadAction>,
    );
    const verb = screen.getByRole("button", { name: "New team" });
    expect(verb.parentElement?.className).toBe("pagehead-action");
  });
});
