/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { project, project360 } from "../../testing/projects.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProjectDetails } from "./projectdetails";

// Each details section offers one Edit, whose dialog holds that section's
// fields and saves through the project's one record write.

afterEach(cleanup);

type Sent = Readonly<{ body: unknown; ifMatch: string | null }>;

function mount(grant: ("read" | "update")[]) {
  const sent: Sent[] = [];
  const bodies: unknown[] = [];
  const current = project({
    description: "Old words",
    target_end_date: "2026-12-01",
  });
  installFetchStub({
    "GET /me": meRoute({ project: grant }, { seat: "full" }),
    "GET /users": () => jsonResponse({ data: [], page: { has_more: false } }),
    "PATCH /projects/pr-1": (body) => {
      bodies.push(body);
      return jsonResponse({ ...current, version: 4 });
    },
  });
  const stubbed = globalThis.fetch;
  const tap: typeof fetch = async (input, init) => {
    const answer = await stubbed(input, init);
    if (input instanceof Request && input.method === "PATCH") {
      sent.push({
        body: bodies.at(-1),
        ifMatch: input.headers.get("If-Match"),
      });
    }
    return answer;
  };
  globalThis.fetch = tap;
  render(
    <StoryProviders>
      <ProjectDetails view={project360({ project: current })} />
    </StoryProviders>,
  );
  return sent;
}

it("offers one Edit per section, and rests every value as a reading", async () => {
  mount(["read", "update"]);
  for (const name of ["Edit About", "Edit Ownership"]) {
    expect(await screen.findByRole("button", { name })).toHaveTextContent(
      en["record.edit"],
    );
  }
  expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
  expect(document.querySelector(".editable-value")).toBeNull();
});

it("offers no Edit to a reader without the update grant", async () => {
  mount(["read"]);
  await screen.findByText("Owner");
  expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
});

it("holds only About's fields and saves the changed description alone, under the version", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  await user.click(await screen.findByRole("button", { name: "Edit About" }));
  const dialog = within(await screen.findByRole("dialog", { name: "About" }));
  expect(dialog.getByLabelText(en["project.targetEnd"])).toBeTruthy();
  expect(dialog.queryByRole("textbox", { name: /^Project name/ })).toBeNull();
  expect(dialog.queryByLabelText(/^Owner/)).toBeNull();
  const description = dialog.getByRole("textbox", { name: "Description" });
  await user.clear(description);
  await user.type(description, "New words");
  await user.click(dialog.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(sent).toEqual([
      { body: { description: "New words" }, ifMatch: "3" },
    ]),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("holds only the owner in the Ownership dialog", async () => {
  const user = userEvent.setup();
  mount(["read", "update"]);
  await user.click(
    await screen.findByRole("button", { name: "Edit Ownership" }),
  );
  const dialog = within(
    await screen.findByRole("dialog", { name: "Ownership" }),
  );
  expect(dialog.getByLabelText(/^Owner/)).toBeTruthy();
  expect(dialog.queryByRole("textbox", { name: "Description" })).toBeNull();
  expect(dialog.queryByLabelText(en["project.targetEnd"])).toBeNull();
});
