/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import "@testing-library/jest-dom/vitest";
import { PageAsideProvider, usePageAside } from "../../app/pageaside";
import { RecordFields } from "../recordfields";
import { StoryProviders } from "../story-utils";
import { DealBrief } from "./dealbrief";

afterEach(cleanup);
function DetailsState() {
  const { open } = usePageAside();
  return open ? (
    <RecordFields
      title="Details"
      kind="deal"
      fields={[{ key: "description", labelText: "Brief", type: "textarea" }]}
      record={{ id: "d1" }}
      canEdit
      save={async () => undefined}
    />
  ) : null;
}
it.each([false, true])(
  "focuses the canonical brief editor when Details initially open is %s",
  async (open) => {
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <PageAsideProvider open={open}>
          <DealBrief brief={null} />
          <DetailsState />
        </PageAsideProvider>
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Write brief" }),
    );
    expect(
      await screen.findByRole("button", { name: "Change Brief" }),
    ).toHaveFocus();
    expect(screen.queryByRole("dialog")).toBeNull();
  },
);
it.each([true, false])(
  "does not offer an inert write action without Details (readOnly=%s)",
  (readOnly) => {
    render(
      <StoryProviders>
        <DealBrief brief={null} readOnly={readOnly} />
      </StoryProviders>,
    );
    expect(screen.getByText(/No brief written yet/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Write brief" })).toBeNull();
  },
);
