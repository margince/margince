/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { type QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { SendMark, SendPermission } from "../components/sendpermission";
import {
  RetractAcquisitionRequest,
  RetractRecordedRequest,
} from "../features/contacts/retractrequest";
import { LocaleProvider } from "../i18n";
import { useSendPermission } from "../lib/usesendpermission";
import {
  allowedPreview,
  refusedPreview,
} from "../testing/sendpermission.testkit";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { createQueryClient } from "./queryclient";

const clients: QueryClient[] = [];
const fetchBefore = globalThis.fetch;
const address = "anna@example.test";
const readyName = "Communication status for this message: Recipients checked";

afterEach(() => {
  cleanup();
  for (const client of clients) client.clear();
  clients.length = 0;
  globalThis.fetch = fetchBefore;
  vi.restoreAllMocks();
});

function CurrentMessage({ recipient = address }: { recipient?: string }) {
  const permission = useSendPermission({
    recipients: [recipient],
    links: [{ entity_type: "contact", entity_id: "contact-one" }],
    context: "requested_followup",
  });
  return (
    <>
      <SendMark {...permission} />
      <SendPermission {...permission} />
    </>
  );
}

it.each([
  { kind: "recorded", pendingInitial: false },
  { kind: "acquisition", pendingInitial: false },
  { kind: "recorded", pendingInitial: true },
  { kind: "acquisition", pendingInitial: true },
])(
  "taking back a request refreshes the current message: $kind, initial check pending: $pendingInitial",
  async ({ kind, pendingInitial }) => {
    let withdrawn = false;
    let finishRecheck: ((answer: Response) => void) | undefined;
    const pendingRecheck = new Promise<Response>((resolve) => {
      finishRecheck = resolve;
    });
    let finishInitial: ((answer: Response) => void) | undefined;
    const initial = new Promise<Response>((resolve) => {
      finishInitial = resolve;
    });
    const preview = vi.fn(() =>
      withdrawn
        ? pendingRecheck
        : pendingInitial
          ? initial
          : jsonResponse(allowedPreview([address])),
    );
    const retractRoute =
      kind === "recorded"
        ? "POST /contacts/contact-one/consent/qualifying-events/request-one/retract"
        : "POST /contacts/contact-one/acquisition-request/retract";
    installFetchStub({
      "POST /emails:preview": preview,
      [retractRoute]: () => {
        withdrawn = true;
        return new Response(null, { status: 204 });
      },
    });
    const client = createQueryClient();
    clients.push(client);
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CurrentMessage />
          {kind === "recorded" ? (
            <RetractRecordedRequest
              contactId="contact-one"
              event={{
                id: "request-one",
                kind: "requested_by_subject",
                occurred_at: "2020-03-02T09:00:00Z",
                note: "Asked by phone",
              }}
              mayWrite
            />
          ) : (
            <RetractAcquisitionRequest
              contactId="contact-one"
              request={{ kinds: ["requested_quote_or_meeting"] }}
              mayWrite
            />
          )}
        </LocaleProvider>
      </QueryClientProvider>,
    );
    if (pendingInitial) {
      await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
    } else {
      expect(
        await screen.findByRole("button", { name: readyName }),
      ).toBeInTheDocument();
    }
    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: "Take back this request" }),
    );
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: "Take back this request",
      }),
    );
    await waitFor(() => expect(withdrawn).toBe(true));
    await waitFor(() => expect(preview).toHaveBeenCalledTimes(2));
    expect(
      screen.getByText("Checking whether this message can be sent…"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: readyName })).toBeNull();
    const answerRecheck = finishRecheck;
    if (!answerRecheck) throw new Error("No preview is waiting");
    await act(async () =>
      answerRecheck(
        jsonResponse(
          refusedPreview(address, {
            reason_code: "no_compatible_evidence",
            decided_by: "machine",
            can_be_overruled: true,
            requested_category: "requested_followup",
          }),
        ),
      ),
    );
    expect(
      await screen.findByText("No recorded reason to contact this recipient"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: readyName })).toBeNull();
    const answerInitial = finishInitial;
    if (!answerInitial) throw new Error("Initial preview is missing");
    await act(async () =>
      answerInitial(jsonResponse(allowedPreview([address]))),
    );
    expect(screen.queryByRole("button", { name: readyName })).toBeNull();
    expect(
      screen.getByText("No recorded reason to contact this recipient"),
    ).toBeInTheDocument();
    expect(preview).toHaveBeenCalledTimes(2);
  },
);

it("keeps the current recipient's refusal when an older response arrives", async () => {
  const nextAddress = "bea@example.test";
  let finishOlder: ((answer: Response) => void) | undefined;
  const older = new Promise<Response>((resolve) => {
    finishOlder = resolve;
  });
  const preview = vi
    .fn()
    .mockImplementationOnce(() => older)
    .mockImplementationOnce(() =>
      jsonResponse(
        refusedPreview(nextAddress, {
          reason_code: "no_compatible_evidence",
          decided_by: "machine",
          can_be_overruled: true,
          requested_category: "requested_followup",
        }),
      ),
    );
  installFetchStub({ "POST /emails:preview": preview });
  const client = createQueryClient();
  clients.push(client);
  const view = (recipient: string) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CurrentMessage recipient={recipient} />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(view(address));
  await waitFor(() => expect(preview).toHaveBeenCalledTimes(1));
  rerender(view(nextAddress));
  expect(
    await screen.findByText("No recorded reason to contact this recipient"),
  ).toBeInTheDocument();
  const answerOlder = finishOlder;
  if (!answerOlder) throw new Error("Older preview is missing");
  await act(async () => answerOlder(jsonResponse(allowedPreview([address]))));
  expect(screen.queryByRole("button", { name: readyName })).toBeNull();
  expect(
    screen.getByText("No recorded reason to contact this recipient"),
  ).toBeInTheDocument();
});

it("removes an earlier ready answer when its recheck fails", async () => {
  const preview = vi
    .fn()
    .mockImplementationOnce(() => jsonResponse(allowedPreview([address])))
    .mockImplementationOnce(() => new Response(null, { status: 400 }));
  installFetchStub({ "POST /emails:preview": preview });
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  const client = createQueryClient();
  clients.push(client);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CurrentMessage />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(
    await screen.findByRole("button", { name: readyName }),
  ).toBeInTheDocument();
  await act(async () =>
    client.invalidateQueries({ queryKey: ["send-permission"] }),
  );
  expect(
    await screen.findByText(/send check did not complete/i),
  ).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: readyName })).toBeNull();
});
