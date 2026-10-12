/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The folder picker: what it offers, whose mailbox it asks, and what it admits
// about a list that stopped short.

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { CaptureExclusionsCard } from "./capture-exclusions";
import {
  backend,
  CAPTURE_EDITOR,
  Providers,
} from "./capture-exclusions.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the container kind", () => {
  // The whole point of the picker: a rule names a provider's own token, and
  // the reader chooses a name they recognise.
  it("offers the mailbox's folders by name and stores the provider's token", async () => {
    const user = userEvent.setup();
    const { fetchMock, calls } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    // The NAME is what the reader sees and picks.
    const picker = await screen.findByRole("combobox");
    await user.click(picker);
    await user.click(await screen.findByRole("option", { name: "Privat" }));
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.add"] }),
    );

    await waitFor(() =>
      expect(calls.some((call) => call.method === "POST")).toBe(true),
    );
    const posted = calls.find((call) => call.method === "POST");
    // The provider-qualified TOKEN is stored, never the display name: a
    // rename must not silently re-point a rule. And the scope is the
    // reader's own, because a label means nothing in anybody else's mailbox.
    expect(posted?.body).toMatchObject({
      kind: "container",
      value: "gmail:Label_7",
      scope: "user",
    });
  });

  // A mailbox that reports no folders is told so, rather than left with a
  // picker that looks broken. The server refuses a container rule it has no
  // token for anyway, so an empty picker would be a dead control.
  it("says so when the mailbox reports no folders", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    // Answer the containers read with an empty list.
    const empty = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.url.includes("/containers")) {
          return new Response(JSON.stringify({ containers: [] }), {
            headers: { "Content-Type": "application/json" },
          });
        }
        return fetchMock(input, init);
      },
    );
    vi.stubGlobal("fetch", empty);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    expect(
      await screen.findByText(en["captureExclusions.noContainers"]),
    ).toBeTruthy();
  });

  // A LIST THAT STOPPED SHORT SAYS SO, beside the folders it did read. Shown as
  // complete, a mailbox whose folder is missing reads as a mailbox without that
  // folder — and somebody stops looking for the thing they came to exclude.
  it("says when the folder list stopped short of the mailbox", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    const partial = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const request =
          input instanceof Request ? input : new Request(String(input), init);
        if (request.url.includes("/containers")) {
          return new Response(
            JSON.stringify({
              containers: [{ id: "f1", name: "Privat" }],
              truncated: true,
            }),
            { headers: { "Content-Type": "application/json" } },
          );
        }
        return fetchMock(input, init);
      },
    );
    vi.stubGlobal("fetch", partial);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    expect(
      await screen.findByText(en["captureExclusions.containersPartial"]),
    ).toBeTruthy();
  });

  // The other half: a mailbox the walk finished makes no such claim, so the
  // note cannot become something every picker carries.
  it("claims nothing when the folder list is whole", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    // `backend` builds the mock and installs nothing, so every case here stubs
    // its own fetch — including this one, whose whole point is the DEFAULT
    // answer.
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );

    await waitFor(() =>
      expect(
        screen.queryByText(en["captureExclusions.containersPartial"]),
      ).toBeNull(),
    );
  });

  // A label lives in ONE mailbox, so there is no workspace choice to offer —
  // the database refuses that rule, and showing the control would offer a
  // scope the write cannot take.
  it("offers no scope choice, because a label is the reader's own", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backend(CAPTURE_EDITOR);
    vi.stubGlobal("fetch", fetchMock);
    render(
      <Providers>
        <CaptureExclusionsCard />
      </Providers>,
    );

    await waitFor(() =>
      expect(screen.getByText("ex@partner.test")).toBeTruthy(),
    );
    await user.click(
      screen.getByRole("button", { name: en["captureExclusions.addOpen"] }),
    );
    // The scope control is there for an address rule...
    expect(
      screen.queryByRole("button", {
        name: en["captureExclusions.scope.workspace"],
      }),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", {
        name: en["captureExclusions.kind.container"],
      }),
    );
    // ...and gone for a container one.
    expect(
      screen.queryByRole("button", {
        name: en["captureExclusions.scope.workspace"],
      }),
    ).toBeNull();
  });
});
