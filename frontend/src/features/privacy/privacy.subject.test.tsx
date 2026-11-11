// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  ANA,
  contactCase,
  resolvedCase,
} from "../../testing/contactcase.fixtures";
import { meFixture } from "../../testing/mefixture";
import { type DataSubjectRequest, DsrDetail } from "./privacy.requests";
import { jsonResponse, render, type Sent, stubRoutes } from "./privacy.testkit";

const ACCESS: DataSubjectRequest = {
  id: "d7",
  kind: "access",
  subject_ref: ANA,
  subject_label: "Mira Kovac",
  subject_kind: "contact",
  status: "in_progress",
  due_at: "2026-08-01T00:00:00Z",
  due_overridden: false,
  dispute_open: false,
  created_at: "2026-07-01T00:00:00Z",
};

const CASE_KEY = `GET /dedupe/contact-cases/${ANA}`;
const PACKAGE_KEY = `GET /data-subject-requests/${ACCESS.id}/package`;
const DOWNLOAD = en["privacy.downloadPackage"];
const HINT = en["privacy.possibleDuplicates"];

function renderDrawer(dsr: DataSubjectRequest) {
  return render(
    <DsrDetail
      dsr={dsr}
      titleId="dsr-title"
      nowMs={Date.parse("2026-07-15T00:00:00Z")}
      onClose={() => {}}
      onFulfilErasure={() => {}}
    />,
  );
}

function noCase() {
  return jsonResponse({ status: 404, title: "Not Found" }, 404);
}

// The names the page handed the browser to save, read off the anchor it clicked.
function savedFiles(): string[] {
  const saved: string[] = [];
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    saved.push(this.download);
  });
  vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:package");
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  return saved;
}

function writesIn(sent: Sent[]) {
  return sent.filter((request) => !request.key.startsWith("GET "));
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the data package on an access request", () => {
  it("downloads the package under the server's filename and writes nothing", async () => {
    const saved = savedFiles();
    const sent = stubRoutes({
      [CASE_KEY]: noCase,
      [PACKAGE_KEY]: () =>
        new Response(JSON.stringify({ contact: { id: ANA } }), {
          status: 200,
          headers: {
            "Content-Type": "application/json",
            "Content-Disposition": 'attachment; filename="access-d7.json"',
          },
        }),
    });
    renderDrawer(ACCESS);

    await userEvent.click(
      await screen.findByRole("button", { name: DOWNLOAD }),
    );

    await waitFor(() => expect(saved).toEqual(["access-d7.json"]));
    expect(sent.filter((request) => request.key === PACKAGE_KEY)).toHaveLength(
      1,
    );
    expect(writesIn(sent)).toEqual([]);
  });

  it("shows the server's refusal and saves nothing", async () => {
    const saved = savedFiles();
    stubRoutes({
      [CASE_KEY]: noCase,
      [PACKAGE_KEY]: () =>
        jsonResponse(
          {
            status: 403,
            title: "Forbidden",
            detail: "Only an administrator can download this package.",
          },
          403,
        ),
    });
    renderDrawer(ACCESS);

    await userEvent.click(
      await screen.findByRole("button", { name: DOWNLOAD }),
    );

    expect(
      await screen.findByText(
        "Only an administrator can download this package.",
      ),
    ).toBeInTheDocument();
    expect(saved).toEqual([]);
  });

  it("is not offered to a seat that cannot work the request", async () => {
    stubRoutes({
      [CASE_KEY]: noCase,
      "GET /me": () =>
        jsonResponse(
          meFixture({
            roles: ["member"],
            allow: { privacy_request: ["read"] },
          }),
        ),
    });
    renderDrawer(ACCESS);

    await screen.findByText(en["privacy.subject"], { selector: "dt" });
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: DOWNLOAD })).toBeNull(),
    );
  });

  it("is not offered on an erasure, whose answer is the erasure itself", async () => {
    stubRoutes({ [CASE_KEY]: noCase });
    renderDrawer({ ...ACCESS, kind: "erasure" });

    expect(
      await screen.findByRole("button", { name: en["privacy.reject"] }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: DOWNLOAD })).toBeNull();
  });
});

describe("the data package for a subject the reader cannot see", () => {
  // An archived or anonymized contact carries no label, but the request still
  // names its id, and the server decides what it can export.
  const UNLABELLED: DataSubjectRequest = {
    ...ACCESS,
    subject_label: null,
    subject_kind: null,
  };

  it("still offers the download for a contact archived after the request, and reads no case", async () => {
    const saved = savedFiles();
    const sent = stubRoutes({
      [PACKAGE_KEY]: () =>
        new Response("{}", {
          status: 200,
          headers: {
            "Content-Type": "application/json",
            "Content-Disposition": 'attachment; filename="access-d7.json"',
          },
        }),
    });
    renderDrawer(UNLABELLED);

    await userEvent.click(
      await screen.findByRole("button", { name: DOWNLOAD }),
    );

    await waitFor(() => expect(saved).toEqual(["access-d7.json"]));
    expect(sent.some((request) => request.key.startsWith("GET /dedupe/"))).toBe(
      false,
    );
  });

  it("shows the server's answer for an anonymized contact as its refusal", async () => {
    const saved = savedFiles();
    stubRoutes({
      [PACKAGE_KEY]: () =>
        jsonResponse(
          {
            status: 404,
            title: "Not Found",
            detail: "This contact no longer exists.",
          },
          404,
        ),
    });
    renderDrawer(UNLABELLED);

    await userEvent.click(
      await screen.findByRole("button", { name: DOWNLOAD }),
    );

    expect(
      await screen.findByText("This contact no longer exists."),
    ).toBeInTheDocument();
    expect(saved).toEqual([]);
  });
});

describe("possible duplicates of a request's subject", () => {
  it("shows the hint and the door to the open case on any kind, and opening writes nothing", async () => {
    const sent = stubRoutes({ [CASE_KEY]: () => jsonResponse(contactCase()) });
    renderDrawer({ ...ACCESS, kind: "erasure" });

    expect(await screen.findByText(HINT)).toBeInTheDocument();
    expect(screen.getByTestId("contact-case-entry")).toHaveTextContent("3");
    expect(writesIn(sent)).toEqual([]);
  });

  it.each([
    ["no case", noCase],
    ["a case already merged", () => jsonResponse(resolvedCase())],
  ])("shows nothing for a contact in %s", async (_, answer) => {
    const sent = stubRoutes({ [CASE_KEY]: answer });
    renderDrawer(ACCESS);

    await screen.findByRole("button", { name: DOWNLOAD });
    await waitFor(() =>
      expect(sent.some((request) => request.key === CASE_KEY)).toBe(true),
    );
    expect(screen.queryByText(HINT)).toBeNull();
    expect(screen.queryByTestId("contact-case-entry")).toBeNull();
  });

  it.each([
    ["a lead", { subject_kind: "lead" as const }],
    [
      "a free-text subject",
      {
        subject_ref: "anna@acme.test",
        subject_label: null,
        subject_kind: null,
      },
    ],
  ])("neither reads a case nor offers the package for %s", async (_, over) => {
    const sent = stubRoutes({ [CASE_KEY]: () => jsonResponse(contactCase()) });
    renderDrawer({ ...ACCESS, ...over });

    // Waits on the seat's grants, which the download needs too.
    await screen.findByRole("button", { name: en["privacy.reject"] });
    expect(screen.queryByRole("button", { name: DOWNLOAD })).toBeNull();
    expect(screen.queryByText(HINT)).toBeNull();
    expect(sent.some((request) => request.key.startsWith("GET /dedupe/"))).toBe(
      false,
    );
  });
});
