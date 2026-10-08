/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { useFocusHandoff } from "./focushandoff";

function Row({ landing }: Readonly<{ landing: () => HTMLElement | null }>) {
  const row = useRef<HTMLDivElement | null>(null);
  useFocusHandoff(row, landing);
  return (
    <div ref={row}>
      <button type="button">Remove</button>
    </div>
  );
}

function Strip({ withRow }: Readonly<{ withRow: boolean }>) {
  const strip = useRef<HTMLDivElement | null>(null);
  return (
    <div ref={strip} tabIndex={-1} data-testid="strip">
      {withRow && <Row landing={() => strip.current} />}
      <button type="button">Elsewhere</button>
    </div>
  );
}

afterEach(cleanup);

describe("useFocusHandoff", () => {
  it("hands the focus a leaving row held to the landing", () => {
    const { rerender } = render(<Strip withRow />);
    screen.getByRole("button", { name: "Remove" }).focus();

    rerender(<Strip withRow={false} />);

    expect(screen.getByTestId("strip")).toHaveFocus();
  });

  it("leaves focus the row did not hold where it is", () => {
    const { rerender } = render(<Strip withRow />);
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
    elsewhere.focus();

    rerender(<Strip withRow={false} />);

    expect(elsewhere).toHaveFocus();
  });

  it("keeps focus on the row while it re-renders with a new landing", () => {
    const { rerender } = render(<Strip withRow />);
    const remove = screen.getByRole("button", { name: "Remove" });
    remove.focus();

    rerender(<Strip withRow />);

    expect(remove).toHaveFocus();
  });
});
