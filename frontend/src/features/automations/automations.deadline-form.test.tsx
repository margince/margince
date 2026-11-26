/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { AutomationDialog } from "./automations.form";

const entry: components["schemas"]["AutomationCatalogEntry"] = {
  key: "route_lead",
  name: "Follow up on new inquiries",
  description: "Create the next task.",
  trigger: "lead.created",
  action: "create_task",
  tier: "auto_execute",
  params_schema: {
    type: "object",
    properties: {
      due_in_days: { type: "integer", default: 1, minimum: 1, maximum: 30 },
      timezone: { type: "string", format: "timezone" },
    },
  },
};

afterEach(cleanup);

function openForm(initialParams: Record<string, unknown>) {
  const submit = vi.fn();
  render(
    <LocaleProvider initial="en">
      <AutomationDialog
        open
        entry={entry}
        initialName="Reply to website inquiries"
        initialParams={initialParams}
        submitLabel="Save"
        pending={false}
        onSubmit={submit}
        onClose={vi.fn()}
      />
    </LocaleProvider>,
  );
  return submit;
}

describe("automation deadline editor", () => {
  it("saves an existing rule without changing its deadline policy", async () => {
    const submit = openForm({ due_in_days: 3 });
    expect(
      screen.getByRole("combobox", { name: "Deadline timezone" }),
    ).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submit).toHaveBeenCalledWith("Reply to website inquiries", {
      due_in_days: 3,
    });
  });
  it("keeps the saved calendar timezone when the rule is edited", async () => {
    const submit = openForm({ due_in_days: 1, timezone: "Europe/Berlin" });
    expect(
      screen.getByRole("combobox", { name: "Deadline timezone" }),
    ).toHaveTextContent("Berlin");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(submit).toHaveBeenCalledWith("Reply to website inquiries", {
      due_in_days: 1,
      timezone: "Europe/Berlin",
    });
  });
});
