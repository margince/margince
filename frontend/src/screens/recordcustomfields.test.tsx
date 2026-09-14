/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import { RecordCustomFields } from "./recordcustomfields";
import { customFieldFixture } from "./recordcustomfields.stories";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

afterEach(cleanup);
it.each(["company", "contact", "deal", "lead"] as const)(
  "shows an unset custom field and sends a sparse %s patch",
  async (kind) => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    const path = kind === "company" ? "companies" : `${kind}s`;
    installFetchStub({
      "GET /me": meRoute(
        { [kind]: ["read", "update"] as const },
        { seat: "full" },
      ),
      "GET /custom-fields": () =>
        jsonResponse({
          data: [
            { ...customFieldFixture, object: kind },
            {
              ...customFieldFixture,
              id: "f2",
              column_name: "cf_other",
              label: "Other",
            },
          ],
          page: { has_more: false },
        }),
      [`PATCH /${path}/record1`]: (body) => {
        sent.push(body);
        return jsonResponse({ id: "record1", version: 2 });
      },
    });
    render(
      <StoryProviders>
        <RecordCustomFields
          kind={kind}
          record={{
            id: "record1",
            version: 1,
            writable: true,
            cf_other: "Keep",
          }}
        />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Change Customer tier" }),
    );
    await user.type(
      screen.getByRole("textbox", { name: "Customer tier" }),
      "Strategic{Enter}",
    );
    await waitFor(() => expect(sent).toEqual([{ cf_tier: "Strategic" }]));
  },
);
it("clears an existing value and never offers a masked field for editing", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "GET /custom-fields": () =>
      jsonResponse({
        data: [
          customFieldFixture,
          {
            ...customFieldFixture,
            id: "f2",
            column_name: "cf_hidden",
            label: "Hidden",
          },
        ],
        page: { has_more: false },
      }),
    "PATCH /contacts/c1": (body) => {
      sent.push(body);
      return jsonResponse({ id: "c1", version: 2 });
    },
  });
  render(
    <StoryProviders>
      <RecordCustomFields
        kind="contact"
        record={{
          id: "c1",
          version: 1,
          writable: true,
          cf_tier: "Strategic",
          masked_fields: ["cf_hidden"],
        }}
      />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Change Customer tier" }),
  );
  await user.clear(screen.getByRole("textbox", { name: "Customer tier" }));
  await user.keyboard("{Enter}");
  await waitFor(() => expect(sent).toEqual([{ cf_tier: null }]));
  expect(screen.queryByRole("button", { name: "Change Hidden" })).toBeNull();
});
