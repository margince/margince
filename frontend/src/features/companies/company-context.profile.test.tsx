// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProblemError } from "../../lib/problem";
import { StoryProviders } from "../../testing/story-utils";
import type { CompanyInput } from "./company-context";
import { CompanyProfileDialog } from "./company-context.profile";

// The company form says on the field what it needs: its own check of the
// required facts before it sends, and the server's limit on the name.

const FORM: CompanyInput = {
  display_name: "Acme",
  website: "acme.test",
  offer_summary: "Pumps",
  icp: "Water utilities",
};

const TOO_LONG = new ProblemError({
  status: 422,
  code: "validation_error",
  detail: "the request failed validation",
  details: {
    errors: [
      {
        field: "display_name",
        code: "too_long",
        message: "a company name is at most 200 characters",
        max: 200,
      },
    ],
  },
});

function Dialog({
  onSubmit,
  refused,
}: Readonly<{ onSubmit: () => void; refused?: ProblemError }>) {
  const [form, setForm] = useState(FORM);
  return (
    <StoryProviders>
      <CompanyProfileDialog
        form={form}
        focus="website"
        pending={false}
        error={refused ?? null}
        refusedAt={refused ? 1 : undefined}
        onChange={setForm}
        onClose={vi.fn()}
        onSubmit={onSubmit}
      />
    </StoryProviders>
  );
}

afterEach(cleanup);

describe("CompanyProfileDialog", () => {
  it("says why an emptied company name cannot be saved, and sends nothing", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<Dialog onSubmit={onSubmit} />);
    const name = await screen.findByRole("textbox", { name: "Company name" });
    expect(name).toBeRequired();
    await user.clear(name);
    await user.click(
      screen.getByRole("button", { name: "Save company context" }),
    );
    expect(await screen.findByText("Company name is required.")).toBeVisible();
    await waitFor(() => expect(name).toHaveFocus());
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("puts the name's limit on the name, focuses it, and drops it once edited", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    const view = render(<Dialog onSubmit={onSubmit} />);
    const name = await screen.findByRole("textbox", { name: "Company name" });
    view.rerender(<Dialog onSubmit={onSubmit} refused={TOO_LONG} />);
    expect(name).toHaveAttribute("maxlength", "200");
    expect(
      await screen.findByText("Company name: at most 200 characters."),
    ).toBeVisible();
    expect(name).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(name).toHaveFocus());
    expect(screen.queryByText(/failed validation/)).toBeNull();
    await user.type(name, "x");
    expect(
      screen.queryByText("Company name: at most 200 characters."),
    ).toBeNull();
  });
});
