/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { row } from "../../testing/worklist.testkit";
import { rowHref } from "./worklist.copy";
import { rowActivityId } from "./worklist.noticesource";
import { WorklistRow } from "./worklist.row";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.useRealTimers();
});
it.each(["task", "meeting", "meeting_outcome", "conversation_claim"] as const)(
  "opens the %s itself with the keyboard and restores focus",
  async (source) => {
    vi.setSystemTime(new Date("2026-07-06T09:00:00Z"));
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/activity-one": () =>
        jsonResponse({
          id: "activity-one",
          kind: source === "task" ? "task" : "meeting",
          subject: "Discuss pricing",
          body: "Check the revised scope before replying.",
          occurred_at: "2026-07-05T09:00:00Z",
          content_state: "available",
          links: [],
        }),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <WorklistRow
          item={row({
            source,
            id: source === "conversation_claim" ? "claim-one" : "activity-one",
            source_record:
              source === "conversation_claim"
                ? {
                    type: "activity",
                    id: "activity-one",
                    label: "Pricing call",
                  }
                : undefined,
            title: "Discuss pricing",
            subject: { type: "contact", id: "contact-one", label: "Mira" },
            actions: [],
          })}
          position={1}
          owner=""
        />
      </StoryProviders>,
    );
    const title = screen.getByRole("button", {
      name: "Discuss pricing · Mira",
    });
    title.focus();
    await user.keyboard("{Enter}");
    expect(
      await screen.findByText("Check the revised scope before replying."),
    ).toBeTruthy();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(title));
  },
);
it("opens explicitly named activity subjects beyond notices without treating other ids as activities", () => {
  expect(
    rowActivityId(
      row({
        source: "conversation_claim",
        subject: { type: "activity", id: "evidence", label: "Pricing note" },
      }),
    ),
  ).toBe("evidence");
  expect(
    rowActivityId(
      row({
        source: "conversation_claim",
        subject: { type: "activity", id: "evidence" },
      }),
    ),
  ).toBe("evidence");
  expect(
    rowActivityId(
      row({
        source: "conversation_claim",
        id: "claim",
        subject: { type: "contact", id: "contact" },
      }),
    ),
  ).toBeUndefined();
});

it("reads an unnamed source and explains a current denial without exposing its id", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/private-evidence": () =>
      jsonResponse(
        { code: "permission_denied", status: 403, title: "Permission denied" },
        403,
      ),
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <WorklistRow
        item={row({
          source: "conversation_claim",
          title: "Review next step",
          subject: { type: "activity", id: "private-evidence" },
          actions: [],
        })}
        position={1}
        owner=""
      />
    </StoryProviders>,
  );
  await user.click(screen.getByRole("button", { name: "Review next step" }));
  expect(await screen.findByText("Activity content is withheld")).toBeTruthy();
  expect(screen.queryByText("private-evidence")).toBeNull();
});

it("opens a commitment's named evidence while retaining its counterparty", () => {
  expect(
    rowActivityId(
      row({
        source: "conversation_claim",
        subject: { type: "contact", id: "buyer", label: "Mira" },
        source_record: {
          type: "activity",
          id: "pricing-call",
          label: "Pricing call",
        },
      }),
    ),
  ).toBe("pricing-call");
  expect(
    rowActivityId(
      row({
        source: "conversation_claim",
        subject: { type: "contact", id: "buyer", label: "Mira" },
        source_record: { type: "activity", id: "private-source" },
      }),
    ),
  ).toBe("private-source");
});

it.each(["default", "compact"] as const)(
  "keeps the record link when a %s title opens source context",
  async (density) => {
    const user = userEvent.setup();
    installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
    const item = row({
      source: "task",
      id: "task-one",
      title: "Send pricing",
      subject: { type: "contact", id: "contact-one", label: "Mira" },
      actions: ["open"],
    });
    render(
      <StoryProviders>
        <WorklistRow
          item={item}
          owner=""
          {...(density === "compact"
            ? { density: "compact" as const }
            : { position: 1 })}
        />
      </StoryProviders>,
    );
    expect(
      screen.getByRole("button", { name: "Send pricing · Mira" }),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: "More actions for Send pricing · Mira",
      }),
    );
    expect(
      screen
        .getAllByRole("link")
        .some((link) => link.getAttribute("href") === `/${rowHref(item)}`),
    ).toBe(true);
  },
);

it.each([false, true])(
  "opens an unnamed source with archived=%s",
  async (archived) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/unnamed-source": () =>
        jsonResponse({
          id: "unnamed-source",
          kind: "note",
          body: "Source context remains readable.",
          occurred_at: "2026-07-01T09:00:00Z",
          content_state: "available",
          links: [],
          archived_at: archived ? "2026-07-01T09:00:00Z" : null,
        }),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <WorklistRow
          item={row({
            source: "conversation_claim",
            title: "Review next step",
            source_record: { type: "activity", id: "unnamed-source" },
            actions: [],
          })}
          position={1}
          owner=""
        />
      </StoryProviders>,
    );
    await user.click(screen.getByRole("button", { name: "Review next step" }));
    expect(
      await screen.findByText("Source context remains readable."),
    ).toBeTruthy();
    expect(Boolean(screen.queryByText("Archived"))).toBe(archived);
    expect(screen.queryByText("unnamed-source")).toBeNull();
  },
);
