/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactFacts } from "./contactfacts";
import { populated } from "./contactpage.fixtures";

function mount(buyingRole: string) {
  render(
    <StoryProviders>
      <ContactFacts contact={populated.contact} buyingRole={buyingRole} />
    </StoryProviders>,
  );
}

beforeEach(() => {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read"] }, { seat: "full" }),
  });
});
afterEach(cleanup);

// The buying role is free text on the wire. A typed one is left out rather
// than shown as a word the reader cannot place.
describe("the buying role on the contact's facts", () => {
  it.each(["contact", "Key contact"])("leaves out a typed %s", (role) => {
    mount(role);
    expect(screen.queryByText(en["contact.page.buyingRole"])).toBeNull();
    expect(screen.queryByText(role)).toBeNull();
  });

  it("names a built-in role however it was capitalised", () => {
    mount(" Champion ");
    expect(screen.getByText(en["contact.page.buyingRole"])).toBeTruthy();
    expect(screen.getByText(en["co.roleLabel.champion"])).toBeTruthy();
  });
});
