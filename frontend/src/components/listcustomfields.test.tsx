/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { en } from "../i18n/en";
import { openViewStep } from "../testing/listview.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import {
  CustomFieldListExample,
  LIST_FIELDS,
} from "./listcustomfields.stories";

afterEach(cleanup);
function show(masked = false) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read"] }),
    "GET /custom-fields": () =>
      jsonResponse({ data: LIST_FIELDS, page: { has_more: false } }),
  });
  render(
    <StoryProviders>
      <CustomFieldListExample masked={masked} />
    </StoryProviders>,
  );
}

it("offers live custom fields as optional columns and displays stored values", async () => {
  const user = userEvent.setup();
  show();
  await waitFor(() =>
    expect(screen.getByText("Dana Buyer")).toBeInTheDocument(),
  );
  await openViewStep(user, "Columns");
  await user.click(
    await screen.findByRole("checkbox", { name: "Customer tier" }),
  );
  expect(
    screen.queryByRole("checkbox", { name: "Retired field" }),
  ).not.toBeInTheDocument();
  await user.keyboard("{Escape}");
  expect(screen.getByText("Gold")).toBeInTheDocument();
});

it("keeps a masked custom value hidden after the reader shows its column", async () => {
  const user = userEvent.setup();
  show(true);
  await openViewStep(user, "Columns");
  await user.click(
    await screen.findByRole("checkbox", { name: "Annual budget" }),
  );
  await user.keyboard("{Escape}");
  expect(screen.getByText(en["record.notShown"])).toBeInTheDocument();
  expect(screen.queryByText("€125.00")).not.toBeInTheDocument();
});

it("applies a catalog picklist filter using its real column key", async () => {
  const user = userEvent.setup();
  show();
  await openViewStep(user, "Filters");
  await user.click(
    await screen.findByRole("button", { name: "Customer tier" }),
  );
  await user.click(screen.getByRole("combobox"));
  await user.click(await screen.findByRole("option", { name: "Gold" }));
  await user.click(screen.getByRole("button", { name: en["cf.filterApply"] }));
  await waitFor(() => expect(window.location.hash).toContain("cf_tier=Gold"));
});
