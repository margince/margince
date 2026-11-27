// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  ToastProvider,
  ToastRegion,
  useToast,
} from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import {
  PICTURE_MAX_BYTES,
  PICTURE_MAX_PIXELS,
  PICTURE_TYPES,
} from "./picturelimits";
import { SHRINKABLE_MAX_BYTES } from "./pictureshrink";
import { ProfilePictureRow, pictureProblem } from "./profilepicture";

type User = components["schemas"]["User"];

const ME = meFixture().user;
const PICTURE = `/v1/users/${ME.id}/avatar?v=2`;
const COLLEAGUE_NAME = ["users", "name", ME.id];

type Sent = { method: string; path: string; body: unknown };

let client: QueryClient;
let sent: Sent[];

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  client.setQueryData(["me"], meFixture());
  client.setQueryData(COLLEAGUE_NAME, { name: ME.display_name });
  sent = [];
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// The server answers each write with `answer`, and every request is kept.
function serve(answer: (method: string) => Response | Promise<Response>) {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const request = input instanceof Request ? input : null;
    const method = request?.method ?? init?.method ?? "GET";
    const url = new URL(request?.url ?? String(input), "http://localhost");
    sent.push({ method, path: url.pathname, body: init?.body });
    return answer(method);
  });
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function Row({ user }: Readonly<{ user: User }>) {
  return <ProfilePictureRow user={user} toast={useToast()} />;
}

function show(user: User = ME) {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <Row user={user} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const picker = () => screen.getByLabelText(englishCopy["account.pictureLabel"]);

const openEdit = (user: UserEvent) =>
  user.click(
    screen.getByRole("button", { name: englishCopy["account.pictureEdit"] }),
  );

it("opens the file chooser from the Edit menu's Upload", async () => {
  serve(() => json(ME));
  const user = userEvent.setup();
  show({ ...ME, avatar_url: PICTURE });
  const opened: HTMLInputElement[] = [];
  const click = vi
    .spyOn(HTMLInputElement.prototype, "click")
    .mockImplementation(function (this: HTMLInputElement) {
      opened.push(this);
    });

  await openEdit(user);
  await user.click(
    screen.getByRole("button", { name: englishCopy["account.pictureAdd"] }),
  );

  expect(opened).toEqual([picker()]);
  click.mockRestore();
});

it("refuses a file that is not PNG, JPEG or WebP before it is sent", async () => {
  serve(() => json(ME));
  const user = userEvent.setup({ applyAccept: false });
  show();
  await user.upload(
    picker(),
    new File(["gif"], "me.gif", { type: "image/gif" }),
  );
  expect(
    await screen.findByText(englishCopy["account.pictureWrongType"]),
  ).toBeInTheDocument();
  expect(sent).toEqual([]);
});

it("refuses a file still over the server's ceiling when the browser cannot shrink it", async () => {
  serve(() => json(ME));
  const user = userEvent.setup();
  show();
  const big = new File([new Uint8Array(PICTURE_MAX_BYTES + 1)], "me.png", {
    type: "image/png",
  });
  await user.upload(picker(), big);
  expect(
    await screen.findByText(englishCopy["account.pictureTooLarge"]),
  ).toBeInTheDocument();
  expect(sent).toEqual([]);
});

it("offers a file at the ceiling and every type the server reads", () => {
  for (const type of PICTURE_TYPES) {
    const atCeiling = new File([new Uint8Array(PICTURE_MAX_BYTES)], "me", {
      type,
    });
    expect(pictureProblem(atCeiling)).toBeNull();
  }
});

it("uploads the picked file and reads every chip of the seat again", async () => {
  serve(() => json({ ...ME, avatar_url: PICTURE }));
  const user = userEvent.setup();
  show();
  const file = new File(["png"], "me.png", { type: "image/png" });
  await user.upload(picker(), file);

  expect(
    await screen.findByText(englishCopy["account.pictureSaved"]),
  ).toBeInTheDocument();
  expect(sent.map(({ method, path }) => `${method} ${path}`)).toEqual([
    "POST /v1/me/avatar",
  ]);
  const body = sent[0]?.body;
  expect(body instanceof FormData ? body.get("file") : null).toBe(file);
  expect(client.getQueryState(["me"])?.isInvalidated).toBe(true);
  expect(client.getQueryState(COLLEAGUE_NAME)?.isInvalidated).toBe(true);
});

it("removes the picture on one press, with no question first", async () => {
  serve(() => json(ME));
  const user = userEvent.setup();
  show({ ...ME, avatar_url: PICTURE });
  await openEdit(user);
  await user.click(
    screen.getByRole("button", { name: englishCopy["account.pictureRemove"] }),
  );

  expect(
    await screen.findByText(englishCopy["account.pictureRemoved"]),
  ).toBeInTheDocument();
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(sent.map(({ method, path }) => `${method} ${path}`)).toEqual([
    "DELETE /v1/me/avatar",
  ]);
  expect(client.getQueryState(["me"])?.isInvalidated).toBe(true);
});

it("offers no removal while the seat shows initials", async () => {
  serve(() => json(ME));
  const user = userEvent.setup();
  show();
  await openEdit(user);
  expect(
    screen.getByRole("button", { name: englishCopy["account.pictureAdd"] }),
  ).toBeVisible();
  expect(
    screen.queryByRole("button", {
      name: englishCopy["account.pictureRemove"],
    }),
  ).toBeNull();
});

it("holds the Edit pill busy while an upload is out", async () => {
  serve(() => new Promise<Response>(() => undefined));
  const user = userEvent.setup();
  show();
  await user.upload(
    picker(),
    new File(["png"], "me.png", { type: "image/png" }),
  );

  const pill = screen.getByRole("button", {
    name: englishCopy["account.pictureEdit"],
  });
  await waitFor(() => expect(pill).toHaveAttribute("aria-busy", "true"));
  expect(picker()).toBeDisabled();
});

it("says in words that the installation cannot store pictures", async () => {
  serve(
    () =>
      new Response(JSON.stringify({ status: 501, code: "not_implemented" }), {
        status: 501,
        headers: { "content-type": "application/problem+json" },
      }),
  );
  const user = userEvent.setup();
  show();
  await user.upload(
    picker(),
    new File(["png"], "me.png", { type: "image/png" }),
  );
  expect(
    await screen.findByText(englishCopy["account.pictureNoStorage"]),
  ).toBeInTheDocument();
  expect(client.getQueryState(["me"])?.isInvalidated).toBe(false);
});

// The browser's decoder and canvas are the boundary. The bitmap has the given size;
// the canvas records its drawing and yields a small blob of the asked type.
function browserDraws(width: number, height: number) {
  const close = vi.fn();
  vi.stubGlobal(
    "createImageBitmap",
    vi.fn(async () => ({ width, height, close })),
  );
  const drawImage = vi.fn();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
    drawImage,
  } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation(
    (done, type) => done(new Blob(["shrunk"], { type })),
  );
  return { drawImage, close };
}

function sentFile(): File | null {
  const body = sent[0]?.body;
  const file = body instanceof FormData ? body.get("file") : null;
  return file instanceof File ? file : null;
}

it("sends a phone photo past the server's pixel bound as its shrunk centre square", async () => {
  const { drawImage, close } = browserDraws(4032, 3024);
  serve(() => json({ ...ME, avatar_url: PICTURE }));
  const user = userEvent.setup();
  show();
  const photo = new File(["jpeg"], "me.jpg", { type: "image/jpeg" });
  await user.upload(picker(), photo);

  expect(
    await screen.findByText(englishCopy["account.pictureSaved"]),
  ).toBeInTheDocument();
  expect(drawImage).toHaveBeenCalledWith(
    expect.anything(),
    504,
    0,
    3024,
    3024,
    0,
    0,
    1024,
    1024,
  );
  const file = sentFile();
  expect(file).not.toBe(photo);
  expect(file?.name).toBe("me.jpg");
  expect(file?.type).toBe("image/jpeg");
  expect(await file?.text()).toBe("shrunk");
  expect(close).toHaveBeenCalled();
});

it("sends a picture at the server's pixel bound as picked", async () => {
  const edge = Math.floor(Math.sqrt(PICTURE_MAX_PIXELS));
  const { drawImage } = browserDraws(edge, edge);
  serve(() => json({ ...ME, avatar_url: PICTURE }));
  const user = userEvent.setup();
  show();
  const photo = new File(["jpeg"], "me.jpg", { type: "image/jpeg" });
  await user.upload(picker(), photo);

  expect(
    await screen.findByText(englishCopy["account.pictureSaved"]),
  ).toBeInTheDocument();
  expect(drawImage).not.toHaveBeenCalled();
  expect(sentFile()).toBe(photo);
});

it("names the pixel bound when the server refuses a picture for its pixels", async () => {
  serve(
    () =>
      new Response(JSON.stringify({ status: 422, code: "too_many_pixels" }), {
        status: 422,
        headers: { "content-type": "application/problem+json" },
      }),
  );
  const user = userEvent.setup();
  show();
  await user.upload(
    picker(),
    new File(["jpeg"], "me.jpg", { type: "image/jpeg" }),
  );
  expect(
    await screen.findByText(englishCopy["account.pictureTooManyPixels"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByText(englishCopy["account.pictureWrongType"]),
  ).toBeNull();
});

it("sends a phone photo over the server's byte ceiling as its shrunk centre square", async () => {
  const { drawImage } = browserDraws(4032, 3024);
  serve(() => json({ ...ME, avatar_url: PICTURE }));
  const user = userEvent.setup();
  show();
  const photo = new File([new Uint8Array(PICTURE_MAX_BYTES + 1)], "me.jpg", {
    type: "image/jpeg",
  });
  await user.upload(picker(), photo);

  expect(
    await screen.findByText(englishCopy["account.pictureSaved"]),
  ).toBeInTheDocument();
  expect(drawImage).toHaveBeenCalled();
  expect(await sentFile()?.text()).toBe("shrunk");
});

it("shrinks a picture within the pixel bound when its file is over the byte ceiling", async () => {
  const { drawImage } = browserDraws(2000, 2000);
  serve(() => json({ ...ME, avatar_url: PICTURE }));
  const user = userEvent.setup();
  show();
  const scan = new File([new Uint8Array(PICTURE_MAX_BYTES + 1)], "me.png", {
    type: "image/png",
  });
  await user.upload(picker(), scan);

  expect(
    await screen.findByText(englishCopy["account.pictureSaved"]),
  ).toBeInTheDocument();
  expect(drawImage).toHaveBeenCalledWith(
    expect.anything(),
    0,
    0,
    2000,
    2000,
    0,
    0,
    1024,
    1024,
  );
  expect(sentFile()?.type).toBe("image/png");
});

it("refuses a file past what the browser decodes before it reads a pixel", async () => {
  browserDraws(4032, 3024);
  serve(() => json(ME));
  const user = userEvent.setup();
  show();
  const huge = new File([new Uint8Array(SHRINKABLE_MAX_BYTES + 1)], "me.jpg", {
    type: "image/jpeg",
  });
  await user.upload(picker(), huge);

  expect(
    await screen.findByText(englishCopy["account.pictureTooLarge"]),
  ).toBeInTheDocument();
  expect(createImageBitmap).not.toHaveBeenCalled();
  expect(sent).toEqual([]);
});
