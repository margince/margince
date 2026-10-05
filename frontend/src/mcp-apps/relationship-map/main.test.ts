// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";
import { relationshipMapFixture } from "./fixture";
import { render } from "./main";

function root(): HTMLElement {
  const el = document.createElement("main");
  el.id = "root";
  document.body.replaceChildren(el);
  return el;
}

function texts(el: HTMLElement, selector: string): (string | null)[] {
  return [...el.querySelectorAll(selector)].map((n) => n.textContent);
}

describe("the relationship map renders what it was given", () => {
  it("renders one row per colleague, in the warmest-first order the seam answered", () => {
    const el = root();
    render(el, relationshipMapFixture.data, []);
    expect(texts(el, ".name")).toEqual([
      "Dana Okafor",
      "Ravi Bhatt",
      "Mira Lindqvist",
    ]);
    expect(texts(el, ".panel-row > .figure")).toEqual(["1", "2", "3"]);
  });

  it("draws each colleague's chip on the mesh keyed by their user id", () => {
    // The app keys a colleague's chip on the seat's user id, so the same
    // colleague is one chip in the app and in a host's panel.
    const el = root();
    render(el, relationshipMapFixture.data, []);
    const chip = el.querySelector<HTMLElement>(".avatar-mesh");
    expect(chip?.textContent).toBe("DO");
    expect(chip?.style.getPropertyValue("--avatar-hue-a")).not.toBe("");
    expect(chip?.getAttribute("aria-hidden")).toBe("true");
  });

  it("speaks the product's strength words over the seam's bands", () => {
    // who_knows answers high / medium / none; every other surface a reader
    // meets says strong / moderate / no contact, with the Routes panel's bars.
    const el = root();
    render(el, relationshipMapFixture.data, []);
    const meters = [...el.querySelectorAll<HTMLElement>(".strength-meter")];
    expect(meters.map((m) => m.dataset.band)).toEqual([
      "strong",
      "moderate",
      "none",
    ]);
    expect(meters.map((m) => m.textContent)).toEqual([
      "strong",
      "moderate",
      "no contact",
    ]);
  });

  it("renders the empty state when nobody here has spoken to the contact", () => {
    const el = root();
    render(el, { contact_id: "p-1", colleagues: [] }, []);
    expect(el.querySelector(".empty")).not.toBeNull();
    expect(el.querySelectorAll(".panel-row")).toHaveLength(0);
  });

  it("shows a never-spoken colleague as absent rather than as a strength of zero", () => {
    // Never having spoken is not a score of zero. Rendering a missing strength
    // as 0 would tell a rep a relationship decayed when none ever existed.
    const el = root();
    render(el, relationshipMapFixture.data, []);
    const cold = el.querySelectorAll(".panel-row")[2];
    expect(cold?.textContent).toContain("no interactions in 90 days");
    expect(cold?.textContent).not.toContain("score");
  });

  it("says the list is not the whole network when the sweep stopped at its bound", () => {
    const el = root();
    render(el, relationshipMapFixture.data, [{ code: "sweep_truncated" }]);
    expect(el.querySelector(".intro")?.textContent).toMatch(
      /not the whole network/i,
    );
    expect(el.querySelector(".panel-head")?.textContent).toContain(
      "at least 3",
    );
  });

  it("claims warmest-first only when the ranking is complete", () => {
    const el = root();
    render(el, relationshipMapFixture.data, []);
    expect(el.querySelector(".intro")?.textContent).toMatch(/warmest first/i);
  });

  it("renders a band the seam has not published yet in its own word, with no meter", () => {
    // The vocabulary belongs to the seam. A view that refused an unknown value
    // would go blank the first time one was added.
    const el = root();
    render(
      el,
      {
        contact_id: "p-1",
        colleagues: [{ display_name: "Sam", strength_bucket: "scorching" }],
      },
      [],
    );
    expect(el.querySelector(".strength-meter")).toBeNull();
    expect(el.textContent).toContain("scorching");
  });

  it("does not read a band name off the prototype chain", () => {
    // A bucket of "constructor" finds a truthy value on any plain object, and a
    // lookup that trusted it would draw a meter for a band that does not exist.
    const el = root();
    render(
      el,
      {
        contact_id: "p-1",
        colleagues: [{ display_name: "Sam", strength_bucket: "constructor" }],
      },
      [],
    );
    expect(el.querySelector(".strength-meter")).toBeNull();
  });

  it("names a colleague by user id when the seam answered no display name", () => {
    const el = root();
    render(
      el,
      {
        contact_id: "p-1",
        colleagues: [{ user_id: "u-77", strength_bucket: "low" }],
      },
      [],
    );
    expect(el.querySelector(".name")?.textContent).toBe("u-77");
  });

  it("renders a missing interaction count as an em dash, never as NaN", () => {
    const el = root();
    render(
      el,
      {
        contact_id: "p-1",
        colleagues: [{ display_name: "Sam", strength_bucket: "low" }],
      },
      [],
    );
    expect(el.textContent).toContain("—");
    expect(el.textContent).not.toContain("NaN");
  });

  it("falls to the empty state on a payload of the wrong shape rather than throwing", () => {
    const el = root();
    expect(() => render(el, 42, [])).not.toThrow();
    expect(el.querySelector(".empty")).not.toBeNull();
  });

  it("says so when the host sent no structured result at all", () => {
    const el = root();
    render(el, null, []);
    expect(el.querySelector(".empty")?.textContent).toMatch(
      /no structured result/i,
    );
  });
});

describe("the panel says who it is about", () => {
  const anchor = "01a0148e-3f66-7206-a206-685f2e40b606";

  it("names the contact instead of printing the id the product calls them", () => {
    const el = root();
    render(
      el,
      {
        contact_id: anchor,
        contact_name: "Marta Vogel",
        colleagues: [{ display_name: "Sam", strength_bucket: "low" }],
      },
      [],
    );
    expect(el.querySelector(".intro")?.textContent).toContain("Marta Vogel");
    expect(el.textContent).not.toContain(anchor);
  });

  it("says this contact rather than falling back to the id", () => {
    const el = root();
    render(
      el,
      {
        contact_id: anchor,
        colleagues: [{ display_name: "Sam", strength_bucket: "low" }],
      },
      [],
    );
    expect(el.querySelector(".intro")?.textContent).toContain("this contact");
    expect(el.textContent).not.toContain(anchor);
  });

  it("counts one colleague in copy rather than a placeholder", () => {
    const el = root();
    render(
      el,
      {
        contact_id: anchor,
        colleagues: [{ display_name: "Sam", strength_bucket: "low" }],
      },
      [],
    );
    const head = el.querySelector(".panel-head")?.textContent ?? "";
    expect(head).toContain("1 colleague");
    expect(head).not.toContain("colleagues");
    expect(head).not.toContain("colleague(s)");
  });

  it("counts several colleagues in the plural", () => {
    const el = root();
    render(
      el,
      {
        contact_id: anchor,
        colleagues: [
          { display_name: "Sam", strength_bucket: "low" },
          { display_name: "Ada", strength_bucket: "low" },
        ],
      },
      [],
    );
    expect(el.querySelector(".panel-head")?.textContent ?? "").toContain(
      "2 colleagues",
    );
  });
});
