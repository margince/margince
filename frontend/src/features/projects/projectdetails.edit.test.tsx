/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
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

// The project's left column changes its fields through the edit dialog's own
// fields and mapping.

afterEach(cleanup);

function mount(grant: ("read" | "update")[]) {
  const sent: unknown[] = [];
  const current = project({ description: "Old words" });
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
    await screen.findByRole("button", { name: "Change Description" }),
  );
  const description = screen.getByRole("textbox", { name: "Description" });
  await user.clear(description);
  await user.type(description, "New words");
  await user.keyboard("{Control>}{Enter}{/Control}");
  await waitFor(() => expect(sent).toEqual([{ description: "New words" }]));
});

it("offers no change to a reader without the update grant", async () => {
  mount(["read"]);
  await screen.findByText("Owner");
  expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
});
