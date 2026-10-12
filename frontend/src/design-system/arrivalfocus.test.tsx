/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useArrivalFocus } from "./arrivalfocus";
import { Heading } from "./heading";

function Page() {
  const title = useArrivalFocus<HTMLHeadingElement>();
  return (
    <Heading size="xlarge" ref={title} tabIndex={-1}>
      Arrived
    </Heading>
  );
}

afterEach(cleanup);

describe("useArrivalFocus", () => {
  it("gives the title the focus the page change dropped on the body", () => {
    render(<Page />);
    expect(screen.getByRole("heading", { name: "Arrived" })).toHaveFocus();
  });

  it("leaves focus where the reader put it", () => {
    const field = document.createElement("input");
    document.body.append(field);
    field.focus();

    render(<Page />);

    expect(field).toHaveFocus();
    field.remove();
  });
});
