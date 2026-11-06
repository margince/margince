// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { readAddressList } from "./composeaddresslist";

describe("readAddressList", () => {
  it("splits on commas, semicolons, spaces, tabs and newlines", () => {
    expect(
      readAddressList(
        "a@one.example, b@two.example; c@three.example d@four.example\te@five.example\nf@six.example",
      ),
    ).toEqual({
      values: [
        "a@one.example",
        "b@two.example",
        "c@three.example",
        "d@four.example",
        "e@five.example",
        "f@six.example",
      ],
      refused: [],
    });
  });

  it("takes the address out of a line copied from Outlook", () => {
    expect(
      readAddressList(
        'Ada Brandt <ada@brandt.example>; "Strojny, Lars" <lars@brandt.example>',
      ),
    ).toEqual({
      values: ["ada@brandt.example", "lars@brandt.example"],
      refused: [],
    });
  });

  it("takes an Outlook line whose name is the address in single quotes", () => {
    expect(
      readAddressList("'lars@brandt.example' <lars@brandt.example>"),
    ).toEqual({ values: ["lars@brandt.example"], refused: [] });
  });

  it("takes a quoted name that holds an @ as a name", () => {
    expect(readAddressList('"Lars @ Brandt" <lars@brandt.example>')).toEqual({
      values: ["lars@brandt.example"],
      refused: [],
    });
  });

  it("ends a single quote at an address, so an apostrophe opens no name", () => {
    expect(readAddressList("'Ada <ada@x.de>, o'brien@y.ie")).toEqual({
      values: ["ada@x.de", "o'brien@y.ie"],
      refused: [],
    });
  });

  it("ends a double quote at a semicolon or a line, so no address is lost in a name", () => {
    expect(readAddressList('"Ada, b@x.de; "Bob" <c@y.de>')).toEqual({
      values: ["b@x.de", "c@y.de"],
      refused: ['" Ada'],
    });
  });

  it("reads an unquoted `Last, First` name before an address as one name", () => {
    expect(
      readAddressList(
        "Doe, John <john@doe.example>; Brandt, Ada <ada@brandt.example>",
      ),
    ).toEqual({
      values: ["john@doe.example", "ada@brandt.example"],
      refused: [],
    });
  });

  it("refuses text more than one comma before a named address", () => {
    expect(readAddressList("stray, Doe, John <john@doe.example>")).toEqual({
      values: ["john@doe.example"],
      refused: ["stray"],
    });
  });

  it("refuses a quoted name a comma sets apart from a named address", () => {
    expect(readAddressList('"Doe, Jane", John <j@x.de>')).toEqual({
      values: ["j@x.de"],
      refused: ['"Doe, Jane"'],
    });
  });

  it("refuses text an empty comma part sets apart from a named address", () => {
    expect(readAddressList("a, , Doe, John <j@x.de>")).toEqual({
      values: ["j@x.de"],
      refused: ["a"],
    });
    expect(readAddressList("a, , John <j@x.de>")).toEqual({
      values: ["j@x.de"],
      refused: ["a"],
    });
  });

  it("refuses a part a comma leaves with no name between it and the address", () => {
    expect(readAddressList("stray, <a@x.de>")).toEqual({
      values: ["a@x.de"],
      refused: ["stray"],
    });
    expect(readAddressList("Doe, Jane <a@x.de>")).toEqual({
      values: ["a@x.de"],
      refused: [],
    });
  });

  it("refuses a comma-split part that no address follows", () => {
    expect(readAddressList("Doe, John")).toEqual({
      values: [],
      refused: ["Doe", "John"],
    });
  });

  it("keeps a comma inside a double-quoted name", () => {
    expect(readAddressList('"Doe, Jane" <jane@doe.example>')).toEqual({
      values: ["jane@doe.example"],
      refused: [],
    });
  });

  it("takes the characters the server takes in a local part", () => {
    expect(
      readAddressList("o'brien@firma.ie a&b@x.de first/last@x.de 100%@x.de"),
    ).toEqual({
      values: ["o'brien@firma.ie", "a&b@x.de", "first/last@x.de", "100%@x.de"],
      refused: [],
    });
  });

  it("refuses a part that is no address and keeps the good ones", () => {
    expect(readAddressList("not-an-address, e@five.example")).toEqual({
      values: ["e@five.example"],
      refused: ["not-an-address"],
    });
  });

  it("refuses a name with no address as one part", () => {
    expect(readAddressList("Ada Brandt; lars@brandt.example")).toEqual({
      values: ["lars@brandt.example"],
      refused: ["Ada Brandt"],
    });
  });

  it("refuses a word between two addresses split by spaces", () => {
    expect(readAddressList("a@one.example word b@two.example")).toEqual({
      values: ["a@one.example", "b@two.example"],
      refused: ["word"],
    });
  });

  it("refuses a bracketed part that is no address, with its name", () => {
    expect(readAddressList("Ada <ada at brandt>")).toEqual({
      values: [],
      refused: ["Ada <ada at brandt>"],
    });
  });

  it("refuses an address with no dot in its domain or two @", () => {
    expect(readAddressList("ada@brandt a@b@c.example").refused).toEqual([
      "ada@brandt",
      "a@b@c.example",
    ]);
  });

  it("reads blank text as nothing", () => {
    expect(readAddressList(" ,; \n")).toEqual({ values: [], refused: [] });
  });
});
