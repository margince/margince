/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { view as liveView } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import {
  archivedContact,
  archivedContactRoutes,
  archiveEntry,
  refusedRead,
} from "./contactarchived.fixtures";
import { ContactPageV2 } from "./contactpage";

afterEach(cleanup);

function mount(routes: RouteMap) {
  installFetchStub(routes);
  render(
    <StoryProviders>
      <ContactPageV2 id="p-1" tab="overview" />
    </StoryProviders>,
  );
}

describe("the contact page on an archived row", () => {
  it("opens read-only: the name, the archived mark, the reason and the history", async () => {
    const asked: string[] = [];
    const routes = archivedContactRoutes();
    for (const read of ["brief", "consent/guard"]) {
      routes[`GET /contacts/p-1/${read}`] = () => {
        asked.push(read);
        return refusedRead();
      };
    }
    mount(routes);

    expect(
      await screen.findByRole("heading", { name: archivedContact.full_name }),
    ).toBeTruthy();
    expect(screen.getByText(en["record.archived"])).toBeTruthy();
    const reason = screen.getByText(en["contact.rail.archivedReadOnly"]);
    const archivedBy = en["history.verb.archive"].replace(
      "{subject}",
      archiveEntry.actor_name ?? archiveEntry.actor_id,
    );
    expect(await screen.findByText(archivedBy)).toBeTruthy();
    expect(
      screen.getByRole("button", { name: en["history.undo.action"] }),
    ).toBeTruthy();

    for (const verb of [en["log.title"], en["contact.action.addTask"]]) {
      const control = screen.getByRole("button", { name: verb });
      expect(control.hasAttribute("disabled")).toBe(true);
      expect(control.getAttribute("aria-describedby")).toBe(reason.id);
    }
    expect(screen.queryByText(en["record.logActivityRefused"])).toBeNull();
    expect(screen.queryByText(en["contact.page.notOpened"])).toBeNull();
    expect(asked).toEqual([]);
  });

  it("refuses the record's verbs in the header and in the menu", async () => {
    const user = userEvent.setup();
    mount(archivedContactRoutes());

    const reason = await screen.findByText(en["contact.rail.archivedReadOnly"]);
    const refused = (control: HTMLElement) => {
      expect(control.hasAttribute("disabled")).toBe(true);
      expect(control.getAttribute("aria-describedby")).toContain(reason.id);
    };
    refused(
      screen.getByRole("button", { name: en["contact.action.meetings"] }),
    );
    await user.click(
      screen.getByRole("button", { name: en["record.moreActions"] }),
    );
    for (const verb of [
      en["contact.action.research"],
      en["lists.addToShortlist"],
    ]) {
      refused(await screen.findByRole("button", { name: verb }));
    }
  });

  it("shows an archived row's address as text, with nothing to press", async () => {
    const routes = archivedContactRoutes();
    routes["GET /contacts/p-1"] = () =>
      jsonResponse({
        ...archivedContact,
        emails: [liveView.contact.emails?.[0]],
        primary_email: liveView.contact.primary_email,
      });
    mount(routes);

    const address = await screen.findByText("dana@brandt.example");
    expect(address.closest("a, button")).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["compose.writeEmail"] }),
    ).toBeNull();
  });

  it("says the contact could not be opened when the row read fails", async () => {
    const routes = archivedContactRoutes();
    routes["GET /contacts/p-1"] = () =>
      jsonResponse({ status: 500, code: "internal" }, 500);
    mount(routes);

    expect(await screen.findByText(en["contact.page.notOpened"])).toBeTruthy();
    expect(screen.queryByText(en["record.archived"])).toBeNull();
  });

  it("turns into the live page once the archive is undone", async () => {
    const user = userEvent.setup();
    let archived = true;
    const routes = archivedContactRoutes();
    routes["GET /contacts/p-1"] = () =>
      jsonResponse(archived ? archivedContact : liveView.contact);
    routes["GET /contacts/p-1/360"] = () =>
      archived ? refusedRead() : jsonResponse(liveView);
    routes["GET /contacts/p-1/brief"] = () =>
      jsonResponse({ contact_id: "p-1", sentences: [], generated_by: "rules" });
    routes["GET /contacts/p-1/consent/guard"] = () =>
      jsonResponse({ contact_id: "p-1", entries: [] });
    routes["POST /records/contact/p-1/history/h-archive/restore"] = () => {
      archived = false;
      return jsonResponse(archiveEntry);
    };
    mount(routes);

    await user.click(
      await screen.findByRole("button", { name: en["history.undo.action"] }),
    );
    const confirm = await screen.findByRole("dialog");
    expect(
      within(confirm).getByText(en["history.undo.confirmArchiveBody"]),
    ).toBeTruthy();
    await user.click(
      within(confirm).getByRole("button", { name: en["history.undo.action"] }),
    );

    expect(
      await screen.findByRole("button", { name: en["tab.overview"] }),
    ).toBeTruthy();
    expect(screen.queryByText(en["record.archived"])).toBeNull();
    expect(screen.queryByText(en["contact.rail.archivedReadOnly"])).toBeNull();
  });
});
