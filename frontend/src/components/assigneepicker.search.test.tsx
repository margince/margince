/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Select } from "../design-system/select";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { TaskAssigneeField, useRecordOwnerOptions } from "./assigneepicker";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Owner({ onChange }: { onChange: (value: string) => void }) {
  const { options } = useRecordOwnerOptions();
  return (
    <Select aria-label="Owner" options={options} value="" onChange={onChange} />
  );
}

it.each(["owner", "assignee"])(
  "finds a real %s option by email",
  async (kind) => {
    const members = Array.from({ length: 14 }, (_, index) => ({
      id: `user-${index}`,
      display_name: `Member ${index}`,
      email: `member${index}@example.test`,
      is_agent: false,
      can_own_records: true,
    }));
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
          "https://test.local",
        );
        return Response.json(
          url.pathname.endsWith("/me")
            ? meFixture()
            : { data: members, page: { has_more: false, next_cursor: null } },
        );
      }),
    );
    const onChange = vi.fn();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          {kind === "owner" ? (
            <Owner onChange={onChange} />
          ) : (
            <TaskAssigneeField active value="" onChange={onChange} />
          )}
        </LocaleProvider>
      </QueryClientProvider>,
    );
    const user = userEvent.setup();
    await screen.findByRole("combobox");
    await vi.waitFor(() => expect(client.isFetching()).toBe(0));
    await user.click(screen.getByRole("combobox"));
    await user.type(
      screen.getByRole("combobox", { name: "Filter the list" }),
      "member13@",
    );
    await user.click(screen.getByRole("option", { name: "Member 13" }));
    expect(onChange).toHaveBeenCalledWith("user-13");
  },
);
