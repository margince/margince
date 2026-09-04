import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { dealRecipientSeat } from "./dealrecipient";

// Who a first message from a deal page is offered to.
//
// A deal has several stakeholders, so this is a CHOICE in a way a lead's
// address is not. The rule is the one a rep makes by hand — champion, else
// somebody the deal is actually talking to, else the first seat — and each
// preference is asserted against a list where the others would win.

type DealCoverageSeat = components["schemas"]["DealCoverageSeat"];

function seat(over: Partial<DealCoverageSeat> & { person_id: string }) {
  return {
    person_name: `Person ${over.person_id}`,
    role: "user",
    engaged: false,
    ...over,
  } satisfies DealCoverageSeat;
}

describe("who a deal's first message is offered to", () => {
  it("takes the champion over an engaged seat and over the first one", () => {
    const seats = [
      seat({ person_id: "p-first", engaged: true }),
      seat({ person_id: "p-champ", role: "champion" }),
    ];
    expect(dealRecipientSeat(seats)?.person_id).toBe("p-champ");
  });

  // A seat somebody has actually spoken with beats one recorded and never
  // contacted: `engaged` means a two-way exchange happened in the window.
  it("takes an engaged seat when no champion is recorded", () => {
    const seats = [
      seat({ person_id: "p-first" }),
      seat({ person_id: "p-talking", engaged: true }),
    ];
    expect(dealRecipientSeat(seats)?.person_id).toBe("p-talking");
  });

  it("falls back to the first seat when nobody is champion or engaged", () => {
    const seats = [seat({ person_id: "p-first" }), seat({ person_id: "p-2" })];
    expect(dealRecipientSeat(seats)?.person_id).toBe("p-first");
  });

  // A null person_name means the caller may not read that person. The seat
  // still counts toward coverage — how many people carry a deal is not being
  // withheld — but addressing a message to them would put a name in the To
  // field that the rest of the product refuses to show this reader.
  it("skips a seat whose person the reader may not see, champion included", () => {
    const seats = [
      seat({ person_id: "p-hidden", role: "champion", person_name: null }),
      seat({ person_id: "p-visible" }),
    ];
    expect(dealRecipientSeat(seats)?.person_id).toBe("p-visible");
  });

  it("offers nobody when every seat is unreadable", () => {
    const seats = [seat({ person_id: "p-hidden", person_name: null })];
    expect(dealRecipientSeat(seats)).toBeUndefined();
  });

  it("offers nobody on a deal with no stakeholders", () => {
    expect(dealRecipientSeat([])).toBeUndefined();
    expect(dealRecipientSeat(undefined)).toBeUndefined();
  });
});
