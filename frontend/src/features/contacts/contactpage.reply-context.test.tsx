/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { mount, quietMoment, view } from "../../testing/contactpage.testkit";
import { jsonResponse } from "../../testing/story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("drafts a recommended follow-up against the cited message instead of the contact", async () => {
  if (!quietMoment) throw new Error("missing quiet moment fixture");
  const user = userEvent.setup();
  const replied = vi.fn(() =>
    jsonResponse({
      subject: "Re: Lieferumfang",
      body: "Hallo Dana, passt der Lieferumfang?",
      ai_generated: true,
    }),
  );
  const newMail = vi.fn(() =>
    jsonResponse({ subject: "Wrong path", body: "Wrong path" }),
  );
  mount(
    "overview",
    {
      ...view,
      moment: {
        ...quietMoment,
        recommended_action: {
          ...quietMoment.recommended_action,
          destination: {
            surface: "composer",
            entity_type: "activity",
            entity_id: "a-1",
            prefill: { intent: "follow_up" },
          },
        },
      },
    },
    [],
    {
      "GET /activities/a-1": () =>
        jsonResponse({
          ...view.activities?.data[0],
          id: "a-1",
          subject: "Lieferumfang",
        }),
      "GET /mail-drafts": () => jsonResponse({ code: "not_found" }, 404),
      "POST /activities/a-1/draft-email": replied,
      "POST /contacts/p-1/draft-email": newMail,
    },
  );
  await user.click(
    await screen.findByRole("button", { name: "Draft a follow-up" }),
  );
  await waitFor(() => expect(replied).toHaveBeenCalledTimes(1));
  expect(newMail).not.toHaveBeenCalled();
  expect(await screen.findByDisplayValue("Re: Lieferumfang")).toBeTruthy();
});

it.each([true, false])(
  "resolves a source outside the timeline, related=%s",
  async (related) => {
    if (!quietMoment) throw new Error("missing quiet moment fixture");
    const user = userEvent.setup();
    const replied = vi.fn(() =>
      jsonResponse({
        subject: "Re: Older source",
        body: "Hallo Dana, passt der Umfang?",
        ai_generated: true,
      }),
    );
    const newMail = vi.fn(() =>
      jsonResponse({ subject: "Wrong", body: "Wrong" }),
    );
    mount(
      "overview",
      {
        ...view,
        activities: { data: [], page: { has_more: true } },
        moment: {
          ...quietMoment,
          recommended_action: {
            ...quietMoment.recommended_action,
            destination: {
              surface: "composer",
              entity_type: "activity",
              entity_id: "older",
              prefill: { intent: "follow_up" },
            },
          },
        },
      },
      [],
      {
        "GET /activities/older": () =>
          jsonResponse({
            ...view.activities?.data[0],
            id: "older",
            subject: "Older source",
            links: [
              {
                entity_type: "contact",
                entity_id: related ? "p-1" : "somebody-else",
              },
            ],
          }),
        "GET /mail-drafts": () => jsonResponse({ code: "not_found" }, 404),
        "POST /activities/older/draft-email": replied,
        "POST /contacts/p-1/draft-email": newMail,
      },
    );
    await user.click(
      await screen.findByRole("button", { name: "Draft a follow-up" }),
    );
    if (related) {
      await waitFor(() => expect(replied).toHaveBeenCalledTimes(1));
      expect(screen.queryByText("Thread cannot be answered")).toBeNull();
    } else {
      await screen.findByText("Thread cannot be answered");
      expect(replied).not.toHaveBeenCalled();
    }
    expect(newMail).not.toHaveBeenCalled();
  },
);

it("plain Email does not generate even while a recommendation is visible", async () => {
  const user = userEvent.setup();
  const generated = vi.fn(() =>
    jsonResponse({ subject: "Unexpected", body: "Unexpected" }),
  );
  mount("overview", { ...view, moment: quietMoment }, [], {
    "POST /contacts/p-1/draft-email": generated,
    "POST /activities/a-1/draft-email": generated,
    "GET /mail-drafts": () => jsonResponse({ code: "not_found" }, 404),
  });
  await user.click(await screen.findByRole("button", { name: "Email" }));
  await screen.findByLabelText("Purpose of the email");
  expect(
    screen
      .getByRole("button", { name: "Draft email" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(generated).not.toHaveBeenCalled();
});
