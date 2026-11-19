/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { project, project360 } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProjectDetails } from "./projectdetails";
import type { Project } from "./projects.form";

// The project's left column changes its fields through the edit dialog's own
// fields and mapping.

afterEach(cleanup);

function mount(
  grant: ("read" | "update")[],
  current: Project = project({ description: "Old words" }),
) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ project: grant }, { seat: "full" }),
    "GET /users": () => jsonResponse({ data: [], page: { has_more: false } }),
    "GET /projects/pr-1/grants": () => jsonResponse({ data: [] }),
    "PATCH /projects/pr-1": (body) => {
      sent.push(body);
      return jsonResponse({ ...current, version: 4 });
    },
  });
  render(
    <StoryProviders>
      <ProjectDetails view={project360({ project: current })} />
    </StoryProviders>,
  );
  return sent;
}

it("saves a changed description from the left column as that field alone", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  await user.click(
    await screen.findByRole("button", { name: /Show \d+ empty fields?/ }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Change Description Old words" }),
  );
  const dialog = await screen.findByRole("dialog", { name: "Description" });
  const description = within(dialog).getByRole("textbox", {
    name: "Description",
  });
  await user.clear(description);
  await user.type(description, "New words");
  await user.keyboard("{Control>}{Enter}{/Control}");
  await waitFor(() => expect(sent).toEqual([{ description: "New words" }]));
});

it("offers no change to a reader without the update grant", async () => {
  mount(["read"]);
  await screen.findByText("Owner");
  expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
  expect(document.querySelector(".editable-value")).toBeNull();
});

it("saves a start date from the Started row as that field alone", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  await user.click(
    await screen.findByRole("button", { name: /Show \d+ empty fields?/ }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Change Started Not set" }),
  );
  const dialog = await screen.findByRole("dialog", { name: "Started" });
  await user.type(within(dialog).getByRole("textbox"), "2026-10-20{Enter}");
  await waitFor(() => expect(sent).toEqual([{ started_at: "2026-10-20" }]));
});

it("offers the end date once the project is closed", async () => {
  mount(
    ["read", "update"],
    project({ phase: "closed", closed_reason: "Done" }),
  );
  await openEmptyFields();
  expect(
    await screen.findByRole("button", { name: "Change Ended Not set" }),
  ).toBeTruthy();
});

async function openEmptyFields() {
  await userEvent
    .setup()
    .click(
      await screen.findByRole("button", { name: /Show \d+ empty fields?/ }),
    );
}
