/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { NewDsrForm } from "./privacy";
import { choose, render, stubRoutes } from "./privacy.testkit";

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("opening a restriction request", () => {
  // Fulfilling it records the restriction on one contact, so the form takes a
  // picked contact and no free-text subject.
  it("requires a picked contact", async () => {
    stubRoutes();
    render(<NewDsrForm onDone={() => {}} />);
    await choose(await screen.findByLabelText(/kind/i), "Restriction");
    expect(screen.getByLabelText(/contact/i)).toBeInTheDocument();
    expect(
      screen.getByText(/restricts the processing of that record/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText(/subject reference/i),
    ).not.toBeInTheDocument();
  });
});
