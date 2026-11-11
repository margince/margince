/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useContactAboutSections } from "./contactdetails";

type Email = components["schemas"]["ContactEmail"];
type Phone = components["schemas"]["ContactPhone"];

afterEach(cleanup);

const stamp = { source: "manual", captured_by: "human:u-1" };
const email = (
  id: string,
  address: string,
  position: number,
  extra: Partial<Email> = {},
): Email => ({
  ...stamp,
  id,
  email: address,
  email_type: "work",
  is_primary: position === 0,
  position,
  ...extra,
});
const phone = (
  id: string,
  number: string,
  position: number,
  extra: Partial<Phone> = {},
): Phone => ({
  ...stamp,
  id,
  phone: number,
  phone_type: "work",
  is_primary: position === 0,
  position,
  ...extra,
});

function About({ page }: Readonly<{ page: typeof view }>) {
  return <RecordDetails sections={useContactAboutSections(page)} />;
}

function show(emails: Email[], phones: Phone[] = []) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read"] }, { seat: "full" }),
  });
  render(
    <StoryProviders>
      <About
        page={{
          ...view,
          contact: {
            ...view.contact,
            emails,
            phones,
            primary_email: emails[0]?.email ?? null,
          },
        }}
      />
    </StoryProviders>,
  );
}

const addresses = () =>
  screen
    .queryAllByRole("link", { name: /@brandt\.example$/ })
    .map((line) => line.textContent);

describe("a contact's addresses and numbers in the About group", () => {
  it("lists every live address with its kind", async () => {
    show([
      email("pe-1", "dana@brandt.example", 0),
      email("pe-2", "home@brandt.example", 1, { email_type: "personal" }),
    ]);
    await screen.findByText("home@brandt.example");
    expect(addresses()).toEqual(["dana@brandt.example", "home@brandt.example"]);
    expect(screen.getByText(en["field.emailPersonal"])).toBeInTheDocument();
  });

  it("never offers an archived address", async () => {
    show([
      email("pe-1", "dana@brandt.example", 0),
      email("pe-2", "old@brandt.example", 1, {
        archived_at: "2026-01-01T00:00:00Z",
      }),
    ]);
    await screen.findByText("dana@brandt.example");
    expect(screen.queryByText("old@brandt.example")).toBeNull();
  });

  it("folds four addresses to two until the reader opens them", async () => {
    const user = userEvent.setup();
    show(
      ["a", "b", "c", "d"].map((name, index) =>
        email(`pe-${name}`, `${name}@brandt.example`, index),
      ),
    );
    await screen.findByText("a@brandt.example");
    expect(addresses()).toEqual(["a@brandt.example", "b@brandt.example"]);
    await user.click(screen.getByRole("button", { name: "+2 more" }));
    expect(addresses()).toHaveLength(4);
    await user.click(
      screen.getByRole("button", { name: en["contact.handles.fewer"] }),
    );
    expect(addresses()).toHaveLength(2);
  });

  it("puts the primary number first, each with its kind", async () => {
    show(
      [email("pe-1", "dana@brandt.example", 0)],
      [
        phone("pp-1", "+493012345678", 0, { is_primary: false }),
        phone("pp-2", "+491701234567", 1, {
          is_primary: true,
          phone_type: "mobile",
        }),
      ],
    );
    await screen.findByText("+491701234567");
    const numbers = screen
      .getAllByRole("link", { name: /^\+49/ })
      .map((line) => line.textContent);
    expect(numbers).toEqual(["+491701234567", "+493012345678"]);
    expect(screen.getByText(en["field.phoneMobile"])).toBeInTheDocument();
  });
});
