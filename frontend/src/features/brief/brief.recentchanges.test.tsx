/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { magicKey } from "../../lib/magic.queries";
import {
  type Call,
  jsonResponse,
  render,
  stubApi,
} from "../../testing/brief.testkit";
import { line, receipt } from "../../testing/magic.fixtures";
import { BriefScreen } from "./brief";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const refused = () => jsonResponse({ title: "Server Error", status: 500 }, 500);

const night = receipt({
  done: [
    line({
      entity: {
        type: "deal",
        id: "00000000-0000-7000-8000-0000000000aa",
        label: "Fleet retrofit",
      },
      undo: {
        undoable: true,
        audit_id: "00000000-0000-7000-8000-0000000000bb",
        version: 7,
      },
    }),
  ],
  needs_you: [
    line({
      id: "00000000-0000-7000-8000-000000000002",
      lane: "needs_you",
      summary: { key: "magic.action.approval_send_email" },
    }),
  ],
});

function doneForYou(): Promise<HTMLElement> {
  return screen.findByRole("region", { name: en["brief.done.title"] });
}

function magicReads(calls: readonly Call[]): number {
  return calls.filter((call) => call.path === "/magic").length;
}

function waitingSection(): HTMLElement | null {
  return screen.queryByRole("region", { name: en["magic.heading.brief"] });
}

it("says once, inside Done for you, that recent changes did not load, and Retry brings them back", async () => {
  let answer = refused;
  stubApi({ "GET /magic": () => answer() });
  const user = userEvent.setup();
  render(<BriefScreen />);
  const done = await doneForYou();
  await within(done).findByText(en["magic.failed"]);
  expect(screen.getAllByText(en["magic.failed"])).toHaveLength(1);
  expect(screen.queryByText(en["state.failed"])).toBeNull();
  const retry = within(done).getAllByRole("button", {
    name: en["state.retry"],
  });
  expect(retry).toHaveLength(1);
  expect(waitingSection()).toBeNull();

  answer = () => jsonResponse(night);
  await user.click(retry[0]);
  expect(await within(done).findByText("1 small change")).toBeTruthy();
  const waiting = await screen.findByRole("region", {
    name: en["magic.heading.brief"],
  });
  expect(
    within(waiting).getByRole("list", { name: en["magic.lane.needsYou"] }),
  ).toBeTruthy();
  expect(screen.queryByText(en["magic.failed"])).toBeNull();
});

it("takes Done for you away when Retry finds a quiet night", async () => {
  let answer = refused;
  stubApi({ "GET /magic": () => answer() });
  const user = userEvent.setup();
  render(<BriefScreen />);
  const done = await doneForYou();
  const retry = await within(done).findByRole("button", {
    name: en["state.retry"],
  });
  answer = () => jsonResponse(receipt());
  await user.click(retry);
  await waitFor(() =>
    expect(
      screen.queryByRole("region", { name: en["brief.done.title"] }),
    ).toBeNull(),
  );
  expect(waitingSection()).toBeNull();
});

it("offers no Undo from the cached receipt once its refresh fails", async () => {
  let answer = () => jsonResponse(night);
  stubApi({ "GET /magic": () => answer() });
  const { client } = render(<BriefScreen />);
  const done = await doneForYou();
  await within(done).findByText("1 small change");
  expect(
    within(done).getAllByRole("button", { name: "Undo", hidden: true }),
  ).toHaveLength(1);

  answer = refused;
  await client.refetchQueries({ queryKey: magicKey });
  await within(done).findByText(en["magic.failed"]);
  expect(
    screen.queryAllByRole("button", { name: "Undo", hidden: true }),
  ).toEqual([]);
  expect(screen.queryByText("1 small change")).toBeNull();
  expect(waitingSection()).toBeNull();
});

it("reads a failed receipt once, though Done for you mounts to say so", async () => {
  const calls = stubApi({ "GET /magic": refused });
  const { client } = render(<BriefScreen />);
  const done = await doneForYou();
  await within(done).findByText(en["magic.failed"]);
  await waitFor(() =>
    expect(client.isFetching({ queryKey: magicKey })).toBe(0),
  );
  expect(magicReads(calls)).toBe(1);
  expect(within(done).getByText(en["magic.failed"])).toBeTruthy();
});

it("keeps the failure and a busy Retry in Done for you while it reads again", async () => {
  let answer: () => Response | Promise<Response> = refused;
  stubApi({ "GET /magic": () => answer() });
  const user = userEvent.setup();
  render(<BriefScreen />);
  const done = await doneForYou();
  const retry = await within(done).findByRole("button", {
    name: en["state.retry"],
  });
  let answerNow: (response: Response) => void = () => undefined;
  answer = () =>
    new Promise<Response>((resolve) => {
      answerNow = resolve;
    });
  await user.click(retry);
  await waitFor(() =>
    expect(
      within(done)
        .getByRole("button", { name: en["state.retry"] })
        .getAttribute("aria-busy"),
    ).toBe("true"),
  );
  expect(within(done).getByText(en["magic.failed"])).toBeTruthy();

  answerNow(jsonResponse(night));
  expect(await within(done).findByText("1 small change")).toBeTruthy();
});

it("reads the receipt again when the reader comes back to Home after a failure", async () => {
  const calls = stubApi({ "GET /magic": refused });
  const { rerender } = render(<BriefScreen />);
  await within(await doneForYou()).findByText(en["magic.failed"]);
  expect(magicReads(calls)).toBe(1);

  rerender(<p>Elsewhere</p>);
  rerender(<BriefScreen />);
  await waitFor(() => expect(magicReads(calls)).toBe(2));
  expect(
    await within(await doneForYou()).findByText(en["magic.failed"]),
  ).toBeTruthy();
});
