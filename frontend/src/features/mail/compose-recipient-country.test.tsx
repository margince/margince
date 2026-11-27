// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en as englishCopy } from "../../i18n/en";
import { isPreviewDoor } from "../../testing/sendpermission.testkit";
import {
  activity202,
  installComposeClock,
  jsonResponse,
  pickWhy,
  pressSend,
  render,
  type Sent,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";
import { ComposeWindow } from "./composewindow";

// A recipient's chip offers the country their marketing rules follow: what the
// record says and where, and the rep's own setting for this one message. It
// rides the send and the saved draft, and a cleared country stays cleared.

const recordCountry = () =>
  jsonResponse({
    recipients: [
      { address: "a@x.com", country: "de", source: "employer_address" },
    ],
  });

const LOOKUP = "POST /recipient-countries";
const count = (sent: Sent[], key: string) =>
  sent.filter((entry) => entry.key === key).length;

function Composer() {
  return (
    <ToastProvider>
      <ComposeWindow
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={() => {}}
      />
      <ToastRegion />
    </ToastProvider>
  );
}

async function openComposer() {
  render(<Composer />);
  await screen.findByRole("combobox", { name: WHY_ASK });
}

async function writeTo(user: UserEvent, address: string) {
  await user.type(screen.getByLabelText("To"), address);
  await user.tab();
  await user.type(screen.getByLabelText("Subject"), "Your quote");
  writeMessage("Body", "The prices you asked for.");
  await pickWhy(user, WHY_LABEL.requestedFollowup);
}

const countryOf = (address: string) =>
  screen.findByRole("button", { name: new RegExp(`Country for ${address}`) });

const sentEmail = (sent: Sent[]) =>
  sent.filter((entry) => entry.key === "POST /emails").at(-1)?.body as
    | Record<string, unknown>
    | undefined;

beforeEach(installComposeClock);

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("a recipient's country on the chip", () => {
  it("reads the record only when the chip is opened, and names its source", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      [LOOKUP]: recordCountry,
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    await openComposer();
    await writeTo(user, "a@x.com");
    expect(count(sent, LOOKUP)).toBe(0);
    await user.click(await countryOf("a@x.com"));
    expect(
      await screen.findByText("Germany, from the employer’s address"),
    ).toBeTruthy();
    expect(count(sent, LOOKUP)).toBe(1);
    expect(sent.filter((entry) => isPreviewDoor(entry.key))).toHaveLength(0);
  });

  it("sends a country the rep set, and a cleared one as null", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      [LOOKUP]: recordCountry,
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    await openComposer();
    await writeTo(user, "a@x.com");
    await user.click(await countryOf("a@x.com"));
    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Country" }),
      "Austria",
    );
    expect(
      await screen.findByText("Austria, your setting for this message"),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: englishCopy["compose.country.clear"],
      }),
    );
    expect(
      await screen.findByText(englishCopy["compose.country.cleared"]),
    ).toBeTruthy();
    await pressSend(user);
    await waitFor(() =>
      expect(sentEmail(sent)?.recipient_countries).toEqual([
        { address: "a@x.com", country: null },
      ]),
    );
  });

  it("sends no countries when the rep goes back to the record's", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const sent = stubRoutes({
      [LOOKUP]: recordCountry,
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    await openComposer();
    await writeTo(user, "a@x.com");
    await user.click(await countryOf("a@x.com"));
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["compose.country.clear"],
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: englishCopy["compose.country.useRecord"],
      }),
    );
    expect(
      await screen.findByText("Germany, from the employer’s address"),
    ).toBeTruthy();
    await pressSend(user);
    await waitFor(() => expect(sentEmail(sent)).toBeDefined());
    expect(sentEmail(sent)?.recipient_countries).toBeUndefined();
  });
});

describe("a recipient's country in the saved draft", () => {
  it("restores a cleared country as cleared and saves it back as null", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const saved = {
      id: "md-1",
      anchor_type: "contact",
      anchor_id: "c-1",
      to: ["a@x.com", "b@x.com"],
      cc: [],
      bcc: [],
      subject: "Your quote",
      body: "The prices you asked for.",
      html_body: "<p>The prices you asked for.</p>",
      recipient_countries: [{ address: "a@x.com", country: null }],
      version: 1,
      agent_drafted: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    const sent = stubRoutes({
      "GET /mail-drafts": () => jsonResponse(saved),
      "PUT /mail-drafts": () => jsonResponse({ ...saved, version: 2 }),
      [LOOKUP]: recordCountry,
    });
    await openComposer();
    await screen.findByText(englishCopy["compose.savedDraftRestored"]);
    await user.click(await countryOf("a@x.com"));
    expect(
      await screen.findByText(englishCopy["compose.country.cleared"]),
    ).toBeTruthy();
    await user.keyboard("{Escape}");
    await user.type(screen.getByLabelText("Subject"), " again");
    await waitFor(() => {
      const last = sent
        .filter((entry) => entry.key === "PUT /mail-drafts")
        .at(-1);
      expect(last?.body).toMatchObject({
        subject: "Your quote again",
        recipient_countries: [{ address: "a@x.com", country: null }],
      });
    });
  });
});
