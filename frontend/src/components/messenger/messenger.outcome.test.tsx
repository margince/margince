/** @vitest-environment happy-dom */
import { en as englishCopy } from "../../i18n/en";
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MessengerOutcome } from "./messenger.outcome";
import { messengerScope } from "./messenger.testkit";

afterEach(cleanup);

it("saves only reviewed outcome text to the selected record without a native transcript or URL", async () => {
  const writes: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
    [`GET /deals/${messengerScope.id}`]: () =>
      jsonResponse({ id: messengerScope.id, name: "Pilot", writable: true }),
    "POST /activities": (body) => {
      writes.push(body);
      return jsonResponse(
        { id: "note-outcome", kind: "note", subject: "Legal review requested" },
        201,
      );
    },
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <MessengerOutcome scope={messengerScope} />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["messenger.shareOutcome"],
    }),
  );
  expect(
    screen.getByText(/Members who can read the destination record/),
  ).toBeVisible();
  expect(screen.queryByLabelText("Subject *")).toBeNull();
  await waitFor(() =>
    expect(
      screen.getByRole("button", {
        name: englishCopy["messenger.writeOutcome"],
      }),
    ).toBeEnabled(),
  );
  await user.click(
    screen.getByRole("button", { name: englishCopy["messenger.writeOutcome"] }),
  );
  expect(
    screen.queryByRole("button", {
      name: englishCopy["messenger.outcomeDestination"],
    }),
  ).toBeNull();
  await user.type(screen.getByLabelText("Subject *"), "Legal review requested");
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    kind: "note",
    source: "manual",
    subject: "Legal review requested",
    links: [{ entity_type: "deal", entity_id: messengerScope.id }],
  });
  expect(writes[0]).not.toHaveProperty("source_system");
  expect(writes[0]).not.toHaveProperty("source_url");
  expect(writes[0]).not.toHaveProperty("source_key");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("checks the selected destination before offering its outcome editor", async () => {
  const company = "018f3a1b-0000-7000-8000-00000000c002";
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
    [`GET /deals/${messengerScope.id}`]: () =>
      jsonResponse({ id: messengerScope.id, name: "Pilot", writable: true }),
    [`GET /companies/${company}`]: () =>
      jsonResponse({
        id: company,
        display_name: "Restricted account",
        writable: false,
      }),
    "GET /search": () =>
      jsonResponse({
        data: [{ type: "company", id: company, title: "Restricted account" }],
      }),
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <MessengerOutcome scope={messengerScope} />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["messenger.shareOutcome"],
    }),
  );
  await user.click(
    screen.getByRole("button", {
      name: englishCopy["messenger.outcomeDestination"],
    }),
  );
  await user.type(screen.getByRole("searchbox"), "Restricted");
  await user.click(
    await screen.findByRole("button", { name: "Restricted account" }),
  );
  expect(
    await screen.findByRole("button", {
      name: englishCopy["messenger.writeOutcome"],
    }),
  ).toBeDisabled();
  expect(screen.queryByLabelText("Subject *")).toBeNull();
});

it("keeps a generated private outcome editable and unsaved until the user logs it", async () => {
  const writes: unknown[] = [];
  const requests: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
    [`GET /deals/${messengerScope.id}`]: () =>
      jsonResponse({ id: messengerScope.id, name: "Pilot", writable: true }),
    [`POST /messenger/slack/deal/${messengerScope.id}/outcome-draft`]: (
      body,
    ) => {
      requests.push(body);
      return jsonResponse({
        subject: "Approval still pending",
        body: "Legal has not approved the pilot.",
      });
    },
    "POST /activities": (body) => {
      writes.push(body);
      return jsonResponse({ id: "reviewed-outcome", kind: "note" }, 201);
    },
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <MessengerOutcome
        scope={messengerScope}
        conversation="C_PRIVATE"
        messageIds={["source-message"]}
      />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["messenger.shareOutcome"],
    }),
  );
  await user.click(
    await screen.findByRole("button", {
      name: englishCopy["messenger.draftOutcome"],
    }),
  );
  const subject = await screen.findByLabelText("Subject *");
  expect(subject).toHaveValue("Approval still pending");
  expect(requests).toEqual([
    { conversation_id: "C_PRIVATE", message_ids: ["source-message"] },
  ]);
  expect(writes).toEqual([]);
  await user.clear(subject);
  await user.type(subject, "Alex to confirm Legal approval");
  await user.click(screen.getByRole("button", { name: "Log" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    subject: "Alex to confirm Legal approval",
    body: "Legal has not approved the pilot.",
  });
});
