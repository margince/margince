// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Fact, RecordFacts } from "./recordfacts";

afterEach(cleanup);

it("renders a cell's label and value", () => {
  render(
    <RecordFacts>
      <Fact label="Owner">Tim Rasche</Fact>
    </RecordFacts>,
  );
  expect(screen.getByText("Owner")).toBeTruthy();
  expect(screen.getByText("Tim Rasche")).toBeTruthy();
});
