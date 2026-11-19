/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { describe, expect, it } from "vitest";
import {
  useRecipientOffer,
  withOffer,
  withoutOffer,
} from "./composerecipientoffer";

describe("the offered recipient", () => {
  it("fills an empty To", () => {
    expect(withOffer([], "dana@brandt.example", false, [])).toEqual([
      "dana@brandt.example",
    ]);
  });

  it("leaves a To the reader filled alone", () => {
    expect(
      withOffer(["cole@nordwand.example"], "dana@brandt.example", false, []),
    ).toEqual(["cole@nordwand.example"]);
  });

  it("is not added beside itself in other capitals", () => {
    expect(
      withOffer(["Dana@Brandt.example"], "dana@brandt.example", true, []),
    ).toEqual(["Dana@Brandt.example"]);
  });

  it("is not added to To while Cc or Bcc holds it, in any case", () => {
    expect(
      withOffer([], "dana@brandt.example", false, ["Dana@Brandt.Example"]),
    ).toEqual([]);
    expect(
      withOffer(["cole@nordwand.example"], "dana@brandt.example", true, [
        "DANA@brandt.example",
      ]),
    ).toEqual(["cole@nordwand.example"]);
  });

  it("fills a vacated slot beside what the reader typed", () => {
    expect(
      withOffer(["cole@nordwand.example"], "dana@brandt.example", true, []),
    ).toEqual(["cole@nordwand.example", "dana@brandt.example"]);
  });

  it("leaves in whatever case it stands when the conversation changes", () => {
    expect(
      withoutOffer(
        ["DANA@brandt.example", "cole@nordwand.example"],
        "dana@brandt.example",
      ),
    ).toEqual(["cole@nordwand.example"]);
  });
});

describe("offering the record's address on a first message", () => {
  function offerInto(copies: readonly string[]): string[] {
    let to: string[] = [];
    const client = new QueryClient();
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, children);
    renderHook(
      () =>
        useRecipientOffer({
          open: true,
          isChannelReply: false,
          answering: undefined,
          recordAddress: "dana@brandt.example",
          to,
          setTo: (update) => {
            to = update(to);
          },
          copies,
        }),
      { wrapper },
    );
    return to;
  }

  it("puts it in To", () => {
    expect(offerInto([])).toEqual(["dana@brandt.example"]);
  });

  it("leaves it where the reader already copied it", () => {
    expect(offerInto(["Dana@Brandt.example"])).toEqual([]);
  });
});

describe("offering the counterparty of a reply", () => {
  it("leaves an address the reader typed when the conversation changes", () => {
    const client = new QueryClient();
    for (const [anchor, address] of [
      ["a-1", "dana@brandt.example"],
      ["a-2", "cole@nordwand.example"],
    ]) {
      client.setQueryData(["compose-reply-recipient", anchor], { address });
    }
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, children);
    let to: string[] = [];
    const copies = ["dana@brandt.example"];
    const { rerender } = renderHook(
      ({ answering }: { answering: string }) =>
        useRecipientOffer({
          open: true,
          isChannelReply: false,
          answering,
          recordAddress: undefined,
          to,
          setTo: (update) => {
            to = update(to);
          },
          copies,
        }),
      { wrapper, initialProps: { answering: "a-1" } },
    );
    // Dana is already in Cc, so the offer does not land.
    expect(to).toEqual([]);

    to = ["Dana@Brandt.example"];
    rerender({ answering: "a-2" });

    expect(to).toEqual(["Dana@Brandt.example"]);
  });
});
