/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { AuditRail } from "./customfields.table";

const wrap = (ui: React.ReactNode) =>
  render(<LocaleProvider initial="en">{ui}</LocaleProvider>);

describe("AuditRail states", () => {
  const noop = () => undefined;

  it("says nothing about emptiness while the read is still running", () => {
    wrap(<AuditRail entries={[]} state="loading" onRetry={noop} />);
    expect(screen.queryByText(/No custom field changes yet/i)).toBeNull();
  });

  it("offers a retry on a failed read, and does not claim the trail is empty", () => {
    const retry = vi.fn();
    wrap(<AuditRail entries={[]} state="failed" onRetry={retry} />);
    expect(screen.getByText(/did not load/i)).toBeInTheDocument();
    expect(screen.queryByText(/No custom field changes yet/i)).toBeNull();
    expect(screen.getByRole("button", { name: /^retry$/i })).toBeTruthy();
  });

  it("says the trail is withheld, not empty, when the role cannot read it", () => {
    wrap(<AuditRail entries={[]} state="withheld" onRetry={noop} />);
    expect(screen.getByText(/Hidden for your role/i)).toBeInTheDocument();
    expect(screen.queryByText(/No custom field changes yet/i)).toBeNull();
  });

  it("shows the empty line only for a settled, genuinely empty read", () => {
    wrap(<AuditRail entries={[]} state="empty" onRetry={noop} />);
    expect(
      screen.getByText(/No custom field changes yet/i),
    ).toBeInTheDocument();
  });
});
