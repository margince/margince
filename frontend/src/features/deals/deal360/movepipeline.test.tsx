/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { ToastProvider, ToastRegion } from "../../../design-system/toast";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { deal } from "../../../testing/brief.fixtures";
import { RecordShell } from "../../../testing/recordshell.testkit";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { DealActions } from "./dealactions";

type Deal = components["schemas"]["Deal"];

const HERE = "018f3a1b-0000-7000-8000-00000000p001";
const STAGE = "018f3a1b-0000-7000-8000-000000000s02";

const pipeline = (id: string, name: string, position: number) => ({
  id,
  name,
  is_default: position === 1,
  position,
  version: 1,
  stages: [],
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(
  allow: Parameters<typeof stubWithSession>[1],
  over: Partial<Deal> = {},
  moves: unknown[] = [],
) {
  stubWithSession(
    {
      "GET /pipelines": () =>
        jsonResponse({
          data: [
            pipeline(HERE, "Sales", 1),
            pipeline("pl-partner", "Partner", 2),
          ],
        }),
      "POST /deals/d-1/move-pipeline": (body) => {
        moves.push(body);
        return jsonResponse({
          ...deal("d-1", "Fleet telematics rollout"),
          version: 2,
        });
      },
    },
    allow,
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <RecordShell>
            <DealActions
              deal={deal("d-1", "Fleet telematics rollout", over)}
              openStages={[]}
              onEditDetails={() => undefined}
            />
          </RecordShell>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function openMore() {
  const user = userEvent.setup();
  await user.click(
    await screen.findByRole("button", { name: en["record.moreActions"] }),
  );
  await screen.findByRole("button", { name: en["log.addTask"] });
  return user;
}

describe("Move to pipeline in the deal's menu", () => {
  it("lists the live pipelines other than the deal's own, moves, and undoes onto the stage it left", async () => {
    const moves: unknown[] = [];
    mount({ deal: ["read", "update"] }, { writable: true }, moves);
    const user = await openMore();

    await user.click(
      await screen.findByRole("button", { name: en["deal.movePipeline"] }),
    );
    expect(await screen.findByText("Partner")).toBeTruthy();
    expect(screen.queryByText("Sales")).toBeNull();

    await user.click(screen.getByText("Partner"));
    await screen.findByText("Moved to Partner.");
    expect(moves).toEqual([{ pipeline_id: "pl-partner" }]);

    await user.click(
      await screen.findByRole("button", { name: en["common.undo"] }),
    );
    await screen.findByText("Moved to Sales.");
    expect(moves[1]).toEqual({ pipeline_id: HERE, stage_id: STAGE });
  });

  it("is absent on a closed deal", async () => {
    mount({ deal: ["read", "update"] }, { status: "won", writable: true });
    await openMore();

    expect(
      screen.queryByRole("button", { name: en["deal.movePipeline"] }),
    ).toBeNull();
  });

  it("is absent for a reader without write on the deal", async () => {
    mount({ deal: ["read"] });
    await openMore();

    expect(
      screen.queryByRole("button", { name: en["deal.movePipeline"] }),
    ).toBeNull();
  });
});
