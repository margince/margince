/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messageText, writeMessage } from "../design-system/richtext-testing";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { LogActivity } from "./logactivity";
import { LogActivityAction } from "./logactivity.action";
import {
  type Captured,
  contact,
  createdActivity,
  jsonResponse,
  READER_ZONE,
  render,
  stubApi,
} from "./logactivity.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const POLISH_ROUTE = "POST /rewrites";
const OWN_SUBJECT = "call anna re pricing";
const OWN_BODY =
  "she wants 40 seats before 12.12.2026, ask about the security sheet";
const POLISHED = {
  subject: "Call Anna about pricing",
  body: "She wants 40 seats before 12.12.2026. Ask whether procurement needs the security sheet.",
};

function polished() {
  return jsonResponse({
    ...POLISHED,
    language: "en",
    ai_disclosure: "Written with AI assistance.",
  });
}

function openForm(
  form: "log" | "task",
  polish: () => Response = polished,
  locale: "en" | "de" = "en",
) {
  const captured: Captured[] = [];
  stubApi(
    {
      "GET /contacts/p1": () => jsonResponse(contact),
      [POLISH_ROUTE]: polish,
      "POST /activities": createdActivity,
    },
    captured,
  );
  render(
    form === "task" ? (
      <LogActivityAction
        entityType="contact"
        entityId="p1"
        askedKind="task"
        openOnMount
      />
    ) : (
      <LogActivity entityType="contact" entityId="p1" />
    ),
    READER_ZONE,
    locale,
  );
  return {
    polishAsks: () => captured.filter((c) => c.key === POLISH_ROUTE),
    logged: () => captured.find((c) => c.key === "POST /activities")?.body,
  };
}

async function typeOwnWords(subjectLabel = "Subject *", bodyLabel = "Details") {
  await userEvent.type(await screen.findByLabelText(subjectLabel), OWN_SUBJECT);
  writeMessage(bodyLabel, OWN_BODY);
}

const polishButton = (name = "Polish wording") =>
  screen.getByRole("button", { name });

describe("Polish wording in Log activity", () => {
  it("offers no action over an empty note", async () => {
    const { polishAsks } = openForm("log");
    await screen.findByLabelText("Subject *");
    expect(screen.queryByRole("button", { name: "Polish wording" })).toBeNull();
    expect(polishAsks()).toHaveLength(0);
  });

  it("sends the rep's own words and language, fills the result in, and saves it marked AI-assisted", async () => {
    const { polishAsks, logged } = openForm("log");
    await typeOwnWords();

    await userEvent.click(polishButton());

    await waitFor(() => expect(messageText("Details")).toBe(POLISHED.body));
    expect(screen.getByLabelText("Subject *")).toHaveProperty(
      "value",
      POLISHED.subject,
    );
    expect(polishAsks()[0]?.body).toEqual({
      envelope: "note",
      verb: "polish",
      subject: OWN_SUBJECT,
      body: OWN_BODY,
      language: "en",
      about: { entity_type: "contact", entity_id: "p1" },
    });
    expect(screen.getByText("AI-assisted")).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() => expect(logged()).toBeDefined());
    expect(logged()).toMatchObject({
      subject: POLISHED.subject,
      body: POLISHED.body,
      ai_assisted: true,
      ai_assisted_on: { entity_type: "contact", entity_id: "p1" },
    });
  });

  it("Undo puts the rep's words back and saves them unmarked", async () => {
    const { logged } = openForm("log");
    await typeOwnWords();
    await userEvent.click(polishButton());
    await waitFor(() => expect(messageText("Details")).toBe(POLISHED.body));

    await userEvent.click(screen.getByRole("button", { name: "Undo" }));

    await waitFor(() => expect(messageText("Details")).toBe(OWN_BODY));
    expect(screen.getByLabelText("Subject *")).toHaveProperty(
      "value",
      OWN_SUBJECT,
    );
    expect(screen.queryByText("AI-assisted")).toBeNull();
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Log" }));
    await waitFor(() => expect(logged()).toBeDefined());
    expect(logged()).toMatchObject({ subject: OWN_SUBJECT, body: OWN_BODY });
    expect(logged()).not.toHaveProperty("ai_assisted");
    expect(logged()).not.toHaveProperty("ai_assisted_on");
  });

  it("withdraws Undo once the rep edits the polished words, and keeps the mark", async () => {
    openForm("log");
    await typeOwnWords();
    await userEvent.click(polishButton());
    await waitFor(() => expect(messageText("Details")).toBe(POLISHED.body));

    await userEvent.type(screen.getByLabelText("Subject *"), " today");

    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
    expect(screen.getByText("AI-assisted")).toBeTruthy();
  });

  it("says why when no model is bound, and keeps the rep's words", async () => {
    const { polishAsks } = openForm("log", () =>
      jsonResponse(
        { status: 409, code: "ai_not_configured", detail: "no model" },
        409,
      ),
    );
    await typeOwnWords();

    await userEvent.click(polishButton());

    const reason = await screen.findByText(en["log.polishNoModel"]);
    expect(reason).toBeTruthy();
    expect(polishButton()).toHaveProperty("disabled", true);
    expect(messageText("Details")).toBe(OWN_BODY);
    expect(screen.queryByText("AI-assisted")).toBeNull();
    expect(polishAsks()).toHaveLength(1);
  });

  it("says why on a record whose contact restricted processing", async () => {
    openForm("log", () =>
      jsonResponse(
        { status: 409, code: "subject_restricted", detail: "restricted" },
        409,
      ),
    );
    await typeOwnWords();

    await userEvent.click(polishButton());

    expect(await screen.findByText(en["log.polishRestricted"])).toBeTruthy();
    expect(polishButton()).toHaveProperty("disabled", true);
  });
});

describe("Polish wording over text too long for one answer", () => {
  it("says so, and offers Polish again once the rep shortens the words", async () => {
    openForm("log", () =>
      jsonResponse(
        {
          status: 422,
          code: "validation_error",
          details: {
            errors: [
              {
                field: "body",
                code: "too_long",
                message: "too long",
                max: 12288,
              },
            ],
          },
        },
        422,
      ),
    );
    await typeOwnWords();

    await userEvent.click(polishButton());

    expect(await screen.findByText(en["log.polishTooLong"])).toBeTruthy();
    expect(polishButton()).toHaveProperty("disabled", true);

    await userEvent.type(screen.getByLabelText("Subject *"), "!");

    expect(polishButton()).toHaveProperty("disabled", false);
  });
});

describe("Polish wording in Add task", () => {
  it("polishes the task's title and saves the task marked AI-assisted", async () => {
    const { polishAsks, logged } = openForm("task");
    await typeOwnWords();

    await userEvent.click(polishButton());

    await waitFor(() =>
      expect(screen.getByLabelText("Subject *")).toHaveProperty(
        "value",
        POLISHED.subject,
      ),
    );
    expect(polishAsks()[0]?.body).toMatchObject({
      subject: OWN_SUBJECT,
      language: "en",
    });
    await userEvent.click(screen.getByRole("button", { name: "Add task" }));
    await waitFor(() => expect(logged()).toBeDefined());
    expect(logged()).toMatchObject({
      kind: "task",
      subject: POLISHED.subject,
      ai_assisted: true,
    });
  });

  it("offers Undo in the task drawer too", async () => {
    openForm("task");
    await typeOwnWords();
    await userEvent.click(polishButton());
    await waitFor(() =>
      expect(screen.getByLabelText("Subject *")).toHaveProperty(
        "value",
        POLISHED.subject,
      ),
    );

    await userEvent.click(screen.getByRole("button", { name: "Undo" }));

    expect(screen.getByLabelText("Subject *")).toHaveProperty(
      "value",
      OWN_SUBJECT,
    );
  });

  it("asks in German for a German reader", async () => {
    const { polishAsks } = openForm("task", polished, "de");
    await typeOwnWords(`${de["log.subject"]} *`, de["log.body"]);

    await userEvent.click(polishButton(de["log.polish"]));

    await waitFor(() => expect(polishAsks()).toHaveLength(1));
    expect(polishAsks()[0]?.body).toMatchObject({
      envelope: "note",
      verb: "polish",
      language: "de",
    });
    expect(polishAsks()[0]?.body).not.toHaveProperty("instruction");
    expect(await screen.findByText(de["co.assistant.aiTag"])).toBeTruthy();
  });
});
