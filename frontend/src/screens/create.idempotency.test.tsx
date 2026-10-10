/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { CreateAction, type CreateField } from "./create";

afterEach(cleanup);

const fields: CreateField[] = [
  { key: "name", labelText: "Name", type: "text" },
];

function mount(
  create: (v: unknown, r: unknown, key: string) => Promise<{ id: string }>,
) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <CreateAction
          label="New thing"
          fields={fields}
          create={create}
          invalidate="things"
          screen="contacts"
          stay
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function fillAndSave(name: string) {
  fireEvent.change(await screen.findByRole("textbox", { name: /Name/ }), {
    target: { value: name },
  });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
}

describe("the key a create form sends", () => {
  it("is the same for a retry after a refusal and new after the record exists", async () => {
    const keys: string[] = [];
    const create = vi.fn(async (_v: unknown, _r: unknown, key: string) => {
      keys.push(key);
      if (keys.length === 1) throw new Error("refused");
      return { id: "c1" };
    });
    mount(create);
    fireEvent.click(screen.getByRole("button", { name: "New thing" }));
    await fillAndSave("Acme");
    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    // A press while the refused save is still latched is swallowed; the one
    // after it lets go goes through.
    await waitFor(() => {
      fireEvent.click(screen.getByRole("button", { name: "Create" }));
      expect(create).toHaveBeenCalledTimes(2);
    });
    expect(keys[1]).toBe(keys[0]);

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(screen.getByRole("button", { name: "New thing" }));
    fireEvent.change(await screen.findByRole("textbox", { name: /Name/ }), {
      target: { value: "Beta" },
    });
    await waitFor(() => {
      fireEvent.click(screen.getByRole("button", { name: "Create" }));
      expect(create).toHaveBeenCalledTimes(3);
    });
    expect(keys[2]).not.toBe(keys[0]);
  });
});
