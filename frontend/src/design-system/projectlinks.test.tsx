/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LocaleProvider } from "../i18n";
import { ProjectLinks, type ProjectLinksAdapter } from "./projectlinks";

afterEach(cleanup);

function draw(adapter: Partial<ProjectLinksAdapter> = {}) {
  const full: ProjectLinksAdapter = {
    linked: [],
    allowsMany: true,
    search: async () => [],
    roles: [{ value: "partner", label: "Partner" }],
    attach: async () => undefined,
    ...adapter,
  };
  render(
    <LocaleProvider initial="en">
      <ProjectLinks
        adapter={full}
        titleKey="companyProjects.title"
        emptyBody="companyProjects.empty"
      />
    </LocaleProvider>,
  );
  return full;
}

describe("ProjectLinks", () => {
  it("says how a link comes to exist when there are none", () => {
    draw();
    expect(screen.getByText("No projects yet")).toBeTruthy();
    // The instructional line, not a bare "nothing here": a reader who cannot
    // see any is the reader who needs telling how one appears.
    expect(screen.getByText(/body of work a deal is about/)).toBeTruthy();
  });

  it("offers to MOVE rather than attach when the record carries at most one", () => {
    draw({
      allowsMany: false,
      linked: [{ project_id: "p1", name: "ERP rollout" }],
    });
    // A deal holds one project, so a second "attach" would promise something
    // the write cannot do.
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(
      screen.getByRole("button", { name: "Move to another project" }),
    ).toBeTruthy();
  });

  it("attaches the picked project and closes", async () => {
    const user = userEvent.setup();
    const attach = vi.fn(async () => undefined);
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout · WR-1" }],
      roles: [
        { value: "customer", label: "Customer" },
        { value: "partner", label: "Partner" },
      ],
      attach,
    });

    await user.click(screen.getByRole("button", { name: "Attach project" }));
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout · WR-1"));

    // With a role picked, the attach carries it — the section never guesses a
    // role, because attaching re-roles an edge that already exists.
    await waitFor(() => expect(attach).toHaveBeenCalledWith("p9", "customer"));
    await waitFor(() =>
      expect(
        screen.queryByRole("searchbox", { name: /Search projects/ }),
      ).toBeNull(),
    );
  });

  it("keeps the dialog open and shows what the server refused", async () => {
    const user = userEvent.setup();
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      attach: async () => {
        throw new Error("this company still has 2 deal(s) on the project");
      },
    });

    await user.click(screen.getByRole("button", { name: "Attach project" }));
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));

    // The refusal is the one moment the reader most needs to see what they
    // asked for, so the dialog stays.
    expect(
      await screen.findByText(/still has 2 deal\(s\) on the project/),
    ).toBeTruthy();
    expect(
      screen.getByRole("searchbox", { name: /Search projects/ }),
    ).toBeTruthy();
  });

  it("asks before detaching, and says nothing is deleted", async () => {
    const user = userEvent.setup();
    const detach = vi.fn(async () => undefined);
    draw({ linked: [{ project_id: "p1", name: "ERP rollout" }], detach });

    await user.click(
      screen.getByRole("button", { name: "Detach ERP rollout" }),
    );
    expect(screen.getByText(/Only its link to this record ends/)).toBeTruthy();
    expect(detach).not.toHaveBeenCalled();

    // The confirm's own verb is distinct from the row's, so a reader — and a
    // screen reader — can tell the ask from the act.
    await user.click(screen.getByRole("button", { name: "Detach it" }));
    await waitFor(() => expect(detach).toHaveBeenCalledWith("p1"));
  });

  it("offers no verbs at all when the reader may not change the links", () => {
    draw({
      readOnly: true,
      linked: [{ project_id: "p1", name: "ERP rollout" }],
      detach: async () => undefined,
      onCreate: () => undefined,
    });
    for (const verb of [
      "New project",
      "Attach project",
      "Detach ERP rollout",
    ]) {
      expect(screen.queryByRole("button", { name: verb })).toBeNull();
    }
  });

  it("does not offer a role when there is only one to hold", () => {
    draw({ roles: [{ value: "partner", label: "Partner" }] });
    // A picker with one option is a question with one answer: it asks the
    // reader to confirm what cannot be otherwise.
    expect(screen.queryByLabelText("As")).toBeNull();
  });

  it("cannot be dismissed while the write is in flight", async () => {
    const user = userEvent.setup();
    let release: (() => void) | undefined;
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      attach: () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    });

    await user.click(screen.getByRole("button", { name: "Attach project" }));
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));

    // Escape while the write is in flight would lose the refusal it is about
    // to return, and the reader would see nothing at all.
    await user.keyboard("{Escape}");
    expect(
      screen.getByRole("searchbox", { name: /Search projects/ }),
    ).toBeTruthy();

    release?.();
    await waitFor(() =>
      expect(
        screen.queryByRole("searchbox", { name: /Search projects/ }),
      ).toBeNull(),
    );
  });
});
