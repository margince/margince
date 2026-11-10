// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// An advice action the server refused this reader is drawn refused, with the
// server's reason, and pressing it writes nothing.

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { company360, companyBackstop } from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { renderSuggestions } from "./company360.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const REFUSED = "You do not have permission to add tasks";

it("draws a refused next step with the server's reason and writes no task", async () => {
  const three60 = {
    ...company360,
    suggestions: [
      {
        kind: "no_next_step",
        fingerprint: "fp-refused-1",
        title: "Agree the rollout date",
        reason: '"PIM rollout" is open and no task says what happens next.',
        evidence: [{ entity_type: "deal", entity_id: "d-1" }],
        action: {
          kind: "add_task",
          deal_id: "d-1",
          task: {
            subject: "Agree the rollout date",
            source: "manual",
            links: [{ entity_type: "deal", entity_id: "d-1" }],
          },
          blocked_reason: REFUSED,
        },
      },
    ],
  };
  stubFetch(companyBackstop, { company360: three60 });
  const requests = vi.spyOn(globalThis, "fetch");
  renderSuggestions(three60 as never);

  const step = await screen.findByRole("button", {
    name: en["co.suggest.act.addTask"],
  });
  expect(step).toBeDisabled();
  expect(step).toHaveAccessibleDescription(REFUSED);
  await userEvent.click(step);
  const posted = requests.mock.calls.filter(([input]) =>
    String(input instanceof Request ? input.url : input).endsWith("/tasks"),
  );
  expect(posted).toEqual([]);
});
