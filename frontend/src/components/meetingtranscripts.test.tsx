// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import { jsonResponse } from "../testing/story-utils";
import { RecordMeetingOutcome } from "./meetingoutcome";

const meeting: components["schemas"]["Activity"] = {
  id: "01a05500-0000-7000-8000-0000000000b1",
  kind: "meeting",
  source: "gcal",
  source_system: "gcal",
  captured_by: "connector:gcal",
  subject: "Renewal review",
  body: "Calendar agenda",
  meeting_notes: "Approved renewal",
  meeting_status: "held",
  calendar_managed: true,
  writable: true,
  occurred_at: "2026-11-26T08:00:00Z",
  created_at: "2026-11-26T08:00:00Z",
  updated_at: "2026-11-26T08:00:00Z",
};
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function setup({
  writable = true,
  refuseCreate = false,
  refuseFile = false,
} = {}) {
  const creates: { key: string | null; body: unknown }[] = [];
  const uploads: FormData[] = [];
  const reads: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(input, init);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/me"))
        return jsonResponse(
          meFixture({ allow: { activity: ["read", "create", "update"] } }),
        );
      if (request.method === "POST" && url.pathname.endsWith("/attachments")) {
        uploads.push(await request.formData());
        return refuseFile && uploads.length === 1
          ? jsonResponse({ title: "Upload refused", status: 400 }, 400)
          : jsonResponse({ id: "file-id" }, 201);
      }
      if (request.method === "POST" && url.pathname.endsWith("/activities")) {
        creates.push({
          key: request.headers.get("Idempotency-Key"),
          body: await request.json(),
        });
        return refuseCreate && creates.length === 1
          ? jsonResponse({ title: "Save failed", status: 500 }, 500)
          : jsonResponse(
              {
                ...meeting,
                id: "01a05500-0000-7000-8000-0000000000b2",
                source_system: "transcript",
              },
              201,
            );
      }
      if (url.pathname.endsWith(`/activities/${meeting.id}`))
        return jsonResponse({ ...meeting, writable });
      if (url.pathname.endsWith("/activities")) reads.push(url);
      return jsonResponse({
        data: [],
        page: { has_more: false, next_cursor: null },
      });
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <LocaleProvider locale="en">
        <ToastProvider>
          <RecordMeetingOutcome
            activity={{ ...meeting, writable }}
            invalidateKeys={[]}
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { creates, uploads, reads };
}

it("reopens a held calendar meeting with its own notes and a transcript editor", async () => {
  const user = userEvent.setup();
  const { creates, reads } = setup();
  await user.click(
    screen.getByRole("button", { name: en["meeting.transcripts"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  expect(await dialog.findByText("Approved renewal")).toBeTruthy();
  expect(dialog.queryByText("Calendar agenda")).toBeNull();
  await user.type(
    dialog.getByRole("textbox", { name: en["meeting.transcriptText"] }),
    "Buyer: approved.",
  );
  const save = dialog.getByRole("button", { name: en["log.save"] });
  save.focus();
  await user.keyboard("{Enter}");
  await waitFor(() => expect(creates).toHaveLength(1));
  expect(creates[0].body).toMatchObject({
    kind: "meeting",
    source_system: "transcript",
    records_meeting_id: meeting.id,
    body: "Buyer: approved.",
  });
  expect(creates[0].key).toBeTruthy();
  await waitFor(() =>
    expect(
      (
        dialog.getByRole("textbox", {
          name: en["meeting.transcriptText"],
        }) as HTMLTextAreaElement
      ).value,
    ).toBe(""),
  );
  expect(
    reads.some(
      (url) =>
        url.searchParams.get("source_activity_id") === meeting.id &&
        url.searchParams.get("kind") === "meeting",
    ),
  ).toBe(true);
});

it("keeps the draft after a failed save and retries with the same meeting and key", async () => {
  const user = userEvent.setup();
  const { creates } = setup({ refuseCreate: true });
  await user.click(
    screen.getByRole("button", { name: en["meeting.transcripts"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  const text = await dialog.findByRole("textbox", {
    name: en["meeting.transcriptText"],
  });
  await user.type(text, "Buyer: send a proposal.");
  await user.click(dialog.getByRole("button", { name: en["log.save"] }));
  await waitFor(() => expect(creates).toHaveLength(1));
  expect((text as HTMLTextAreaElement).value).toBe("Buyer: send a proposal.");
  await waitFor(() =>
    expect(
      (
        dialog.getByRole("button", {
          name: en["log.save"],
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  await user.click(dialog.getByRole("button", { name: en["log.save"] }));
  await waitFor(() => expect(creates).toHaveLength(2));
  expect(creates[1]).toEqual(creates[0]);
});

it("retries an original file upload without creating a second transcript", async () => {
  const user = userEvent.setup();
  const { creates, uploads } = setup({ refuseFile: true });
  await user.click(
    screen.getByRole("button", { name: en["meeting.transcripts"] }),
  );
  let dialog = within(await screen.findByRole("dialog"));
  const picker = await dialog.findByLabelText(en["log.transcriptUpload"]);
  const file = new File(["Buyer: approved."], "review.txt", {
    type: "text/plain",
  });
  Object.defineProperty(file, "text", {
    value: async () => "Buyer: approved.",
  });
  fireEvent.change(picker, { target: { files: [file] } });
  await waitFor(() =>
    expect(
      (
        dialog.getByRole("textbox", {
          name: en["meeting.transcriptText"],
        }) as HTMLTextAreaElement
      ).value,
    ).toBe("Buyer: approved."),
  );
  await user.click(dialog.getByRole("button", { name: en["log.save"] }));
  await dialog.findByText(en["meeting.transcriptFileRetry"]);
  await user.click(dialog.getAllByRole("button", { name: en["common.close"] })[0]);
  await user.click(
    screen.getByRole("button", { name: en["meeting.transcripts"] }),
  );
  dialog = within(await screen.findByRole("dialog"));
  await dialog.findByText(en["meeting.transcriptFileRetry"]);
  await user.click(dialog.getByRole("button", { name: en["log.save"] }));
  await waitFor(() => expect(uploads).toHaveLength(2));
  expect(creates).toHaveLength(1);
  expect(
    uploads.map((form) => [form.get("entity_type"), form.get("entity_id")]),
  ).toEqual([
    ["activity", meeting.id],
    ["activity", meeting.id],
  ]);
});

it("explains read-only access while keeping the meeting notes readable", async () => {
  const user = userEvent.setup();
  setup({ writable: false });
  await user.click(
    screen.getByRole("button", { name: en["meeting.transcripts"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  expect(await dialog.findByText(en["meeting.readOnly"])).toBeTruthy();
  expect(dialog.getByText("Approved renewal")).toBeTruthy();
  expect(dialog.queryByRole("textbox")).toBeNull();
});

it("waits for a picked transcript to finish reading before enabling save", async () => {
  const user = userEvent.setup();
  const { creates } = setup();
  await user.click(
    screen.getByRole("button", { name: en["meeting.transcripts"] }),
  );
  const dialog = within(await screen.findByRole("dialog"));
  await user.type(
    await dialog.findByRole("textbox", { name: en["meeting.transcriptText"] }),
    "Earlier draft",
  );
  let finish: (text: string) => void = () => {};
  const file = new File(["File transcript"], "review.txt", {
    type: "text/plain",
  });
  Object.defineProperty(file, "text", {
    value: () =>
      new Promise<string>((resolve) => {
        finish = resolve;
      }),
  });
  fireEvent.change(dialog.getByLabelText(en["log.transcriptUpload"]), {
    target: { files: [file] },
  });
  await waitFor(() =>
    expect(
      (
        dialog.getByRole("button", {
          name: en["log.save"],
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true),
  );
  expect(creates).toHaveLength(0);
  finish("File transcript");
  await waitFor(() =>
    expect(
      (
        dialog.getByRole("button", {
          name: en["log.save"],
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false),
  );
  await user.click(dialog.getByRole("button", { name: en["log.save"] }));
  await waitFor(() => expect(creates).toHaveLength(1));
  expect(creates[0].body).toMatchObject({ body: "File transcript" });
});
