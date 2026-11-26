/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import { listsMe, shortlist } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactRail } from "./contactrail";

afterEach(cleanup);

describe("the Lists group", () => {
  function serve(me: () => Response, onLists: readonly unknown[] = []) {
    installFetchStub({
      "GET /me": me,
      [`GET /records/contact/${view.contact.id}/lists`]: () =>
        jsonResponse({ data: onLists }),
    });
    render(
      <StoryProviders>
        <ContactRail
          view={view}
          consent={{
            guard: undefined,
            loading: false,
            failed: false,
            onRetry: () => undefined,
          }}
          writeRefusalId="write-refused"
        />
      </StoryProviders>,
    );
  }

  const group = async () =>
    within(
      await screen.findByRole("region", { name: en["lists.record.title"] }),
    );

  it("offers Add to Shortlist and every list while the contact is on none", async () => {
    serve(listsMe(true));
    const lists = await group();
    expect(
      await lists.findByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeTruthy();
    expect(
      lists
        .getByRole("link", { name: en["lists.record.allLists"] })
        .getAttribute("href"),
    ).toBe("/#/filters/lists");
  });

  it("keeps the lists it is on beside the verbs", async () => {
    serve(listsMe(true), [shortlist]);
    const lists = await group();
    expect(
      await lists.findByRole("button", { name: shortlist.name }),
    ).toBeTruthy();
    expect(
      lists.getByRole("button", { name: en["lists.addToShortlist"] }),
    ).toBeTruthy();
  });
});
