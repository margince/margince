// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ContractForm } from "./contractform";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubApi(): Request[] {
  const writes: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.method === "POST") {
        writes.push(request);
        return Response.json({ id: "c-1" }, { status: 201 });
      }
      if (new URL(request.url).pathname.endsWith("/installation/settings")) {
        return Response.json({ base_currency: "EUR" });
      }
      return Response.json({ data: [] });
    }),
  );
  return writes;
}

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContractForm companyId="o-1" open onClose={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// A browser's Enter submits a field's form through its default button, which
// here sits in the pinned foot and joins the form by its `form` attribute.
async function fieldsForm() {
  const title = await screen.findByLabelText(/^Title/);
  const save = screen.getByRole("button", { name: "Record contract" });
  const form = (title as HTMLInputElement).form;
  expect(form).not.toBeNull();
  expect((save as HTMLButtonElement).form).toBe(form);
  expect(save).toHaveAttribute("type", "submit");
  return { title, form: form as HTMLFormElement };
}

it("records the agreement when the reader submits from a field", async () => {
  const writes = stubApi();
  show();
  const { title, form } = await fieldsForm();
  await userEvent.type(title, "MSA");
  fireEvent.submit(form);

  await waitFor(() => expect(writes).toHaveLength(1));
  expect(await writes[0]?.json()).toMatchObject({ title: "MSA" });
});

it("sends nothing on a submit while the form refuses its own draft", async () => {
  const writes = stubApi();
  show();
  const { title, form } = await fieldsForm();
  fireEvent.submit(form);
  await userEvent.type(title, "M");
  fireEvent.submit(form);

  await waitFor(() => expect(writes).toHaveLength(1));
  expect(await writes[0]?.json()).toMatchObject({ title: "M" });
});
