// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { CompanyLogo } from "./companylogo";

afterEach(cleanup);

it("keeps the fallback until the logo paints", () => {
  const { container } = render(
    <CompanyLogo name="Acme" src="/acme.png" fallback={<span>AC</span>} />,
  );
  expect(screen.getByText("AC")).toBeTruthy();
  const image = container.querySelector("img");
  if (!image) throw new Error("the logo image was not rendered");
  fireEvent.load(image);
  expect(screen.queryByText("AC")).toBeNull();
  expect(screen.getByRole("img", { name: "Acme" })).toBeTruthy();
});

it("restores the fallback when the logo cannot be drawn", () => {
  const { container } = render(
    <CompanyLogo name="Acme" src="/broken.png" fallback={<span>AC</span>} />,
  );
  const image = container.querySelector("img");
  if (!image) throw new Error("the logo image was not rendered");
  fireEvent.error(image);
  expect(screen.getByText("AC")).toBeTruthy();
  expect(container.querySelector("img")).toBeNull();
  expect(screen.getByRole("img", { name: "Acme" })).toBeTruthy();
});
