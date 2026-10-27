/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";

// The deal's left column changes its fields through the details drawer's own
// fields and save.

const deal: components["schemas"]["Deal"] = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: true,
  version: 4,
  pipeline_id: "p1",
  stage_id: "s1",
  currency: "EUR",
  amount_minor: 4800000,
  source: "manual",
  captured_by: "human:u",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

afterEach(cleanup);

function mount(grant: ("read" | "update")[], current = deal) {
  const sent: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: grant }, { seat: "full" }),
    "PATCH /deals/d1": (body) => {
      sent.push(body);
      return jsonResponse({ ...current, version: 5 });
    },
  });
  render(
    <StoryProviders>
      <DealAside deal={current} stages={[]} />
    </StoryProviders>,
  );
  return sent;
}

it("saves a changed value from the left column's money form as that field alone", async () => {
  const user = userEvent.setup();
  const sent = mount(["read", "update"]);
  await user.click(await screen.findByRole("button", { name: "Change Value" }));
  const amount = screen.getByRole("spinbutton", { name: "Value" });
  await user.clear(amount);
  await user.type(amount, "52000");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toEqual([{ amount_minor: 5200000 }]));
});

it("offers no change to a reader without the update grant", async () => {
  mount(["read"]);
  await screen.findByText("Value");
  expect(screen.queryByRole("button", { name: "Change Value" })).toBeNull();
});
