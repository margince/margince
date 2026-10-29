/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import {
  MARKER,
  newClient,
  renderWith,
  repSeeing,
  TWO_MARKS,
} from "../testing/shellharness";
import { WorkspaceRail } from "./shell";

// The sidebar's head: whose company this is, drawn from /me's
// installation_brand for every seat, and the product's own mark when /me
// carries none. How the head behaves on a section level is rail.test.tsx's.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const ABSURD_NAME =
  "Bundesverband der mittelständischen Industrie- und Handelsunternehmen für erneuerbare Energien e. V.";

// The happy-dom render lays nothing out, so the two widths the tip reads
// are stubbed to say whether the name fit its one line.
function nameFits(fits: boolean) {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
    fits ? 120 : 640,
  );
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(160);
}

describe("The rail's brand block", () => {
  // The company is the heading. The product it runs on is the line beneath
  // it, outside the link.
  it("heads the rail with the company's square mark and puts the product under it", () => {
    const client = newClient();
    client.setQueryData(["me"], repSeeing(TWO_MARKS));
    renderWith(client, <WorkspaceRail route={{ screen: "home" }} />);
    const brand = screen.getByRole("link", {
      name: "Demo GmbH home, powered by Margince",
    });
    expect(brand.textContent).toBe("Demo GmbH");
    expect(document.querySelector(".ws-chip img")?.getAttribute("src")).toBe(
      TWO_MARKS.logo_icon_url,
    );
    // The attribution and the build badge stand beside the link. Neither is
    // somewhere a press should lead.
    const attribution = document.querySelector(".ws-company");
    expect(attribution?.textContent).toContain("Powered by Margince");
    expect(within(brand).queryByText("Powered by Margince")).toBeNull();
    expect(brand.getAttribute("href")).toBe("#/home");
  });

  // A name of any length keeps the head one line tall, so the whole name has
  // to be reachable some other way, and only when the line cut it.
  it.each([
    ["shows the whole name when its line cut it", false, ABSURD_NAME],
    ["stays away when the name fits", true, null],
  ])("%s", (_name, fits, shown) => {
    nameFits(fits);
    const client = newClient();
    client.setQueryData(["me"], repSeeing({ display_name: ABSURD_NAME }));
    renderWith(client, <WorkspaceRail route={{ screen: "home" }} />);

    fireEvent.focus(screen.getByRole("link", { name: /Bundesverband/ }));

    expect(screen.queryByRole("tooltip")?.textContent ?? null).toBe(shown);
  });

  // A company save refreshes /me (storeCompany). The rail has to be OBSERVING
  // the entry rather than peeking at it once: a peek left the old face in the
  // rail until something unrelated re-rendered the shell, so a contact who had
  // just uploaded a mark saw the monogram stay.
  it("re-draws the head when /me arrives with a new mark", async () => {
    const client = newClient();
    const profile = {
      display_name: "Demo GmbH",
    };
    client.setQueryData(["me"], repSeeing(profile));
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "home" }} />,
    );
    expect(container.querySelector(".ws-chip img")).toBeNull();

    act(() => {
      client.setQueryData(
        ["me"],
        repSeeing({
          ...profile,
          logo_icon_url:
            "/v1/companies/44444444-4444-4444-8444-444444444444/logo/icon",
        }),
      );
    });
    await waitFor(() =>
      expect(container.querySelector(".ws-chip img")?.getAttribute("src")).toBe(
        "/v1/companies/44444444-4444-4444-8444-444444444444/logo/icon",
      ),
    );
  });

  // /me before the installation described itself carries no brand, and the
  // head is the product's own rather than an invented company.
  it("draws the product's own mark when /me carries no brand", () => {
    const client = newClient();
    client.setQueryData(["me"], repSeeing());
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "home" }} />,
    );
    expect(
      screen.getByRole("link", { name: en["shell.logoAria"] }),
    ).toBeTruthy();
    expect(container.querySelector(".ws-chip-company")).toBeNull();
    expect(container.querySelector(".ws-name")?.textContent).toBe(
      en["shell.logoAria"],
    );
  });

  // A company whose site declared no icon has a face rather than a gap. The
  // monogram is the floor under the mark, so the slot is never empty.
  it("draws the company's monogram when no logo resolved", () => {
    const client = newClient();
    client.setQueryData(
      ["me"],
      repSeeing({
        display_name: "Demo GmbH",
      }),
    );
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "home" }} />,
    );
    expect(container.querySelector(".ws-chip img")).toBeNull();
    expect(container.querySelector(".ws-chip .avatar")?.textContent).toBe("DG");
  });

  // Every brand branch carries it — the product's own mark, an installation's
  // logo, an installation with only a monogram. The marker is a fact about the
  // BUILD, so it does not depend on whether there is a company name above it,
  // and the branch with nothing to attribute is the one a first-run reader sees.
  it.each([
    ["the product's own mark", undefined],
    ["an installation's logo", TWO_MARKS],
    [
      "an installation's monogram",
      { ...TWO_MARKS, logo_url: undefined, logo_icon_url: undefined },
    ],
  ])("stamps the stage on the head with %s", (_name, company) => {
    const client = newClient();
    if (company) {
      client.setQueryData(["me"], repSeeing(company));
    }
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "home" }} />,
    );
    expect(container.querySelectorAll(MARKER)).toHaveLength(1);
    expect(container.querySelector(MARKER)?.textContent).toBe(en["shell.beta"]);
  });

  // One shape at both widths: the head never swaps marks when the panel
  // collapses, so the company keeps one face.
  it.each([
    ["expanded", false],
    ["collapsed", true],
  ])("draws the square icon when the panel is %s", (_name, collapsed) => {
    const client = newClient();
    client.setQueryData(["me"], repSeeing(TWO_MARKS));
    const { container } = renderWith(
      client,
      <WorkspaceRail route={{ screen: "home" }} collapsed={collapsed} />,
    );
    expect(container.querySelectorAll(".ws-chip img")).toHaveLength(1);
    expect(container.querySelector(".ws-chip img")?.getAttribute("src")).toBe(
      TWO_MARKS.logo_icon_url,
    );
  });

  // The site read stores a square lockup in the wide slot alone. A square
  // there is the company's mark; a wordmark squeezed into one is illegible.
  it.each([
    ["a square lockup", 200, 200, TWO_MARKS.logo_url],
    ["a wordmark", 700, 200, undefined],
  ])(
    "draws %s from the wide slot only when it is square",
    async (_name, width, height, drawn) => {
      vi.stubGlobal("Image", imageOfSize(width, height));
      const client = newClient();
      client.setQueryData(
        ["me"],
        repSeeing({ ...TWO_MARKS, logo_icon_url: undefined }),
      );
      const { container } = renderWith(
        client,
        <WorkspaceRail route={{ screen: "home" }} />,
      );
      await waitFor(() =>
        expect(
          container.querySelector(".ws-chip img")?.getAttribute("src") ??
            undefined,
        ).toBe(drawn),
      );
      if (!drawn) {
        expect(container.querySelector(".ws-chip .avatar")?.textContent).toBe(
          "DG",
        );
      }
    },
  );
});

// An Image whose every load succeeds at one size, so the probe's answer is the
// case's and not the test environment's network.
function imageOfSize(width: number, height: number) {
  return class {
    naturalWidth = width;
    naturalHeight = height;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(_value: string) {
      queueMicrotask(() => this.onload?.());
    }
  };
}
