/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { steppedClock } from "../testing/steppedclock";
import {
  ProjectLinks,
  type ProjectLinksAdapter,
  RemoveLinkMenu,
} from "./projectlinks";

afterEach(cleanup);

// The dialog's own confirm, not the section's verb of the same name behind it.
function confirmAttach() {
  return within(screen.getByRole("dialog")).getByRole("button", {
    name: englishCopy["projectLinks.attach"],
  });
}

// The row's verb: its overflow menu, named for the record, then Remove.
async function openRemove(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await user.click(
    screen.getByRole("button", { name: `More actions for ${name}` }),
  );
  await user.click(screen.getByRole("button", { name: "Remove" }));
}

function draw(adapter: Partial<ProjectLinksAdapter> = {}, bare = false) {
  const full: ProjectLinksAdapter = {
    linked: [],
    allowsMany: true,
    search: async () => [],
    roles: [{ value: "partner", label: "Partner" }],
    attach: async () => undefined,
    ...adapter,
  };
  const view = render(
    <LocaleProvider initial="en">
      <ProjectLinks
        adapter={full}
        titleKey="companyProjects.title"
        emptyBody="companyProjects.empty"
        bare={bare}
      />
    </LocaleProvider>,
  );
  return { ...full, container: view.container };
}

describe("ProjectLinks", () => {
  it("says how a link comes to exist when there are none", () => {
    draw();
    // One quiet line, no display heading over it: as a pane of its own the
    // section already carries its name in the head.
    expect(
      screen.queryByText(englishCopy["projectLinks.emptyTitle"]),
    ).toBeNull();
    // The instructional line, not a bare "nothing here": a reader who cannot
    // see any is the reader who needs telling how one appears.
    expect(screen.getByText(/once it is on a project/)).toBeTruthy();
    // And that line alone. As a pane of its own the section already carries
    // its name in the head, so a heading under it says the same word twice
    // and makes an absence the loudest thing in the column.
    expect(
      screen.queryByText(englishCopy["projectLinks.emptyTitle"]),
    ).toBeNull();
  });

  // Bare, the section is a GROUP inside a pane the caller holds: no pane of
  // its own, the title as a group head one level down with the verb beside
  // it, and the empty state as the plate an empty group draws — titled there,
  // because a group head is not a pane head and the plate is what carries the
  // absence.
  it("stands as a group inside a pane when the caller holds the pane", () => {
    const { container } = draw({}, true);
    expect(container.querySelector(".panel")).toBeNull();
    expect(container.querySelector(".panel-grouphead h3")?.textContent).toBe(
      "Projects",
    );
    expect(
      container.querySelector(".panel-grouphead button")?.textContent,
    ).toBe(englishCopy["projectLinks.attach"]);
    expect(container.querySelector(".empty-plate")).toBeTruthy();
    expect(
      screen.getByText(englishCopy["projectLinks.emptyTitle"]),
    ).toBeTruthy();
  });

  it("offers to MOVE rather than attach when the record carries at most one", () => {
    draw({
      allowsMany: false,
      linked: [{ project_id: "p1", name: "ERP rollout" }],
    });
    // A deal holds one project, so a second "attach" would promise something
    // the write cannot do.
    expect(
      screen.queryByRole("button", {
        name: englishCopy["projectLinks.attach"],
      }),
    ).toBeNull();
    expect(
      screen.getByRole("button", { name: englishCopy["projectLinks.move"] }),
    ).toBeTruthy();
  });

  it("attaches the picked project and closes", async () => {
    const user = steppedClock();
    const attach = vi.fn(async () => undefined);
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout · WR-1" }],
      roles: [
        { value: "customer", label: "Customer" },
        { value: "partner", label: "Partner" },
      ],
      attach,
    });

    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout · WR-1"));

    // The pick only selects: the role is still the reader's to choose, so
    // nothing is written until the confirm.
    expect(attach).not.toHaveBeenCalled();
    await user.click(confirmAttach());

    // With a role picked, the attach carries it — the section never guesses a
    // role, because attaching re-roles an edge that already exists.
    await waitFor(() => expect(attach).toHaveBeenCalledWith("p9", "customer"));
    await waitFor(() =>
      expect(
        screen.queryByRole("searchbox", { name: /Search projects/ }),
      ).toBeNull(),
    );
  });

  it("writes the role chosen after the pick, and confirms nothing unpicked", async () => {
    const user = steppedClock();
    const attach = vi.fn(async () => undefined);
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      roles: [
        { value: "customer", label: "Customer" },
        { value: "partner", label: "Partner" },
      ],
      attach,
    });

    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    expect(confirmAttach().hasAttribute("disabled")).toBe(true);
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));
    expect(confirmAttach().hasAttribute("disabled")).toBe(false);

    await user.click(screen.getByRole("combobox", { name: "As" }));
    await user.click(await screen.findByRole("option", { name: "Partner" }));
    await user.click(confirmAttach());

    await waitFor(() => expect(attach).toHaveBeenCalledWith("p9", "partner"));
  });

  it("keeps the dialog open and shows what the server refused", async () => {
    const user = steppedClock();
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      // Every real adapter refuses through throwProblem (companyprojects and
      // projectcompanies both do), so the stand-in refuses the
      // same way. A plain Error here would be a test supplying its own version
      // of production and proving nothing about it.
      attach: async () => {
        throw new ProblemError({
          status: 404,
          code: "not_found",
          detail: "not found",
        });
      },
    });

    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));
    await user.click(confirmAttach());

    // The refusal is the one moment the reader most needs to see what they
    // asked for, so the dialog stays.
    expect(await screen.findByText("not found")).toBeTruthy();
    expect(
      screen.getByRole("searchbox", { name: /Search projects/ }),
    ).toBeTruthy();
  });

  it("opens each dialog without the refusal another one met", async () => {
    const user = steppedClock();
    const refused = new ProblemError({
      status: 404,
      code: "not_found",
      detail: "not found",
    });
    draw({
      linked: [{ project_id: "p1", name: "ERP rollout" }],
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      attach: async () => {
        throw refused;
      },
      detach: async () => {
        throw refused;
      },
    });

    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));
    await user.click(confirmAttach());
    expect(await screen.findByText("not found")).toBeTruthy();
    await user.keyboard("{Escape}");

    await openRemove(user, "ERP rollout");
    expect(screen.getByText(/Only its link to this record ends/)).toBeTruthy();
    expect(screen.queryByText("not found")).toBeNull();

    // And the other way round: a refused detach says nothing to the next
    // attach.
    await user.click(
      screen.getByRole("button", {
        name: englishCopy["projectLinks.detachConfirm"],
      }),
    );
    expect(await screen.findByText("not found")).toBeTruthy();
    await user.keyboard("{Escape}");
    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    expect(
      screen.getByRole("searchbox", { name: /Search projects/ }),
    ).toBeTruthy();
    expect(screen.queryByText("not found")).toBeNull();
  });

  it("asks before detaching, and says nothing is deleted", async () => {
    const user = userEvent.setup();
    const detach = vi.fn(async () => undefined);
    draw({ linked: [{ project_id: "p1", name: "ERP rollout" }], detach });

    await openRemove(user, "ERP rollout");
    expect(screen.getByText(/Only its link to this record ends/)).toBeTruthy();
    expect(detach).not.toHaveBeenCalled();

    // The confirm's own verb is distinct from the row's, so a reader — and a
    // screen reader — can tell the ask from the act.
    await user.click(
      screen.getByRole("button", {
        name: englishCopy["projectLinks.detachConfirm"],
      }),
    );
    await waitFor(() => expect(detach).toHaveBeenCalledWith("p1"));
  });

  it("offers no verbs at all when the reader may not change the links", () => {
    draw({
      readOnly: true,
      linked: [{ project_id: "p1", name: "ERP rollout" }],
      detach: async () => undefined,
      create: <button type="button">New project</button>,
    });
    for (const verb of [
      "New project",
      "Attach project",
      "More actions for ERP rollout",
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
    const user = steppedClock();
    let release: (() => void) | undefined;
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      attach: () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    });

    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));
    await user.click(confirmAttach());

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

  it("says what it links, so a mirror cannot half-rename itself", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <ProjectLinks
          adapter={{
            linked: [{ project_id: "o1", name: "Beta Systeme" }],
            allowsMany: true,
            search: async () => [],
            roles: [{ value: "partner", label: "Partner" }],
            attach: async () => undefined,
            detach: async () => undefined,
          }}
          titleKey="projectCompanies.title"
          emptyBody="projectCompanies.empty"
          words={{
            attach: "Attach company",
            move: "Attach company",
            detachTitle: "Remove company from project?",
            detachConfirm: "Remove company",
            search: "Search companies by name",
          }}
        />
      </LocaleProvider>,
    );
    // The project page's mirror links COMPANIES: a verb reading "Attach
    // project" there is wrong on screen and wrong in the dialog's accessible
    // name, which is what a screen reader announces.
    expect(
      screen.getByRole("button", {
        name: englishCopy["projectCompanies.attach"],
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["projectLinks.attach"],
      }),
    ).toBeNull();
    await openRemove(user, "Beta Systeme");
    expect(
      screen.getByRole("button", {
        name: englishCopy["projectCompanies.detachConfirm"],
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: englishCopy["projectLinks.detachConfirm"],
      }),
    ).toBeNull();
  });

  // The disclosure this section used to make. A reader who may see a company
  // but not attach it to a project is refused by `auth.Require`, whose detail
  // is the RBAC object and the verb.
  it("answers a permission refusal in the reader's words, never the server's", async () => {
    const user = steppedClock();
    draw({
      search: async () => [{ id: "p9", name: "Warehouse rollout" }],
      attach: async () => {
        throw new ProblemError({
          code: "permission_denied",
          detail: "company.link_project: permission denied",
        });
      },
    });

    await user.click(
      screen.getByRole("button", { name: englishCopy["projectLinks.attach"] }),
    );
    await user.type(
      screen.getByRole("searchbox", { name: /Search projects/ }),
      "ware",
    );
    await user.click(await screen.findByText("Warehouse rollout"));
    await user.click(confirmAttach());

    expect(await screen.findByText(/do not have permission/)).toBeTruthy();
    expect(screen.queryByText(/company\.link_project/)).toBeNull();
  });

  // A caller that supplies `card` owns how a row looks. The section still owns
  // when the verb exists, so a read-only record gives the card no way to detach.
  it("hands a supplied card its detach verb only while the record is writable", () => {
    const card: ProjectLinksAdapter["card"] = (linked, onDetach) => (
      <p>{`${linked.name}:${onDetach ? "can-detach" : "fixed"}`}</p>
    );
    const linked = [{ project_id: "c1", name: "Netcare" }];
    const detach = async () => undefined;

    draw({ linked, card, detach });
    expect(screen.getByText("Netcare:can-detach")).toBeTruthy();
    cleanup();

    draw({ linked, card, detach, readOnly: true });
    expect(screen.getByText("Netcare:fixed")).toBeTruthy();
  });

  it("draws the key the caller hands it beside the name", () => {
    draw({
      linked: [
        {
          project_id: "p1",
          name: "ERP rollout",
          key: <span data-testid="caller-key">ERP-1</span>,
        },
      ],
    });
    expect(screen.getByTestId("caller-key").textContent).toBe("ERP-1");
  });

  // Each row's menu is reached from the keyboard alone, and Escape puts the
  // reader back on its trigger without removing anything.
  it("opens the row's menu from the keyboard and closes it on Escape", async () => {
    const user = userEvent.setup();
    const detach = vi.fn(async () => undefined);
    draw({ linked: [{ project_id: "p1", name: "ERP rollout" }], detach });

    const trigger = screen.getByRole("button", {
      name: "More actions for ERP rollout",
    });
    trigger.focus();
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Remove" })).toBeTruthy();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(detach).not.toHaveBeenCalled();
  });
});

describe("RemoveLinkMenu", () => {
  it("names its menu for the record, or generically when it has no name", () => {
    render(
      <LocaleProvider initial="en">
        <RemoveLinkMenu name="Brandt Automotive GmbH" onRemove={() => {}} />
        <RemoveLinkMenu name={null} onRemove={() => {}} />
      </LocaleProvider>,
    );
    expect(
      screen.getByRole("button", {
        name: "More actions for Brandt Automotive GmbH",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: englishCopy["record.moreActions"] }),
    ).toBeTruthy();
  });
});
