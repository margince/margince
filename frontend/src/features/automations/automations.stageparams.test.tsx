/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption, toggleOptions } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { AutomationDialog } from "./automations.form";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("clears destination stages when the chosen pipeline changes and saves the selected IDs", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [
              {
                id: "pipeline-a",
                name: "Sales",
                is_default: true,
                position: 0,
                stages: [
                  {
                    id: "stage-a",
                    name: "Proposal",
                    semantic: "open",
                    win_probability: 50,
                    pipeline_id: "pipeline-a",
                    position: 0,
                  },
                ],
              },
              {
                id: "pipeline-b",
                name: "Renewals",
                is_default: false,
                position: 1,
                stages: [
                  {
                    id: "stage-b",
                    name: "Review",
                    semantic: "open",
                    win_probability: 50,
                    pipeline_id: "pipeline-b",
                    position: 0,
                  },
                ],
              },
            ],
          }),
          { headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
  const submit = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AutomationDialog
          open
          entry={{
            key: "stage_change_notify",
            name: "Stage notice",
            trigger: "deal.stage_changed",
            action: "notify",
            params_schema: {
              type: "object",
              properties: {
                pipeline_id: { type: "string", format: "pipeline" },
                stage_ids: { type: "array", format: "stages" },
              },
            },
          }}
          initialName="Stage notice"
          initialParams={{ pipeline_id: "pipeline-a", stage_ids: ["stage-a"] }}
          submitLabel="Save"
          pending={false}
          onSubmit={submit}
          onClose={vi.fn()}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const pipeline = await screen.findByRole("combobox", { name: "Pipeline" });
  await pickOption(user, pipeline, "Renewals");
  const stages = screen.getByRole("combobox", { name: "Destination stages" });
  expect(stages).not.toHaveTextContent("Proposal");
  await toggleOptions(user, stages, ["Review"]);
  await user.click(screen.getByRole("button", { name: "Save" }));
  expect(submit).toHaveBeenCalledWith("Stage notice", {
    pipeline_id: "pipeline-b",
    stage_ids: ["stage-b"],
  });
});
