/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { TokenSuggestion } from "../../design-system/tokeninput";
import { LocaleProvider } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { AddressBlock, type AddressLine } from "./composehead";

afterEach(cleanup);

const contact = (index: number): TokenSuggestion => ({
  value: `buyer${index}@brandt.example`,
  label: `Buyer ${index} Brandt`,
  hint: `buyer${index}@brandt.example`,
});

const CONTACTS = Array.from({ length: 10 }, (_, index) => contact(index + 1));

const COLLEAGUES: readonly TokenSuggestion[] = [
  {
    value: "sofia@margince.example",
    label: "Sofia Keller",
    hint: "sofia@margince.example",
  },
];

function Head({
  start = [],
  openCopies = false,
  locale = "en",
}: Readonly<{ start?: string[]; openCopies?: boolean; locale?: Locale }>) {
  const [to, setTo] = useState<string[]>(start);
  const [cc, setCc] = useState<string[]>([]);
  const [bcc, setBcc] = useState<string[]>([]);
  const [copiesOpen, setCopiesOpen] = useState(openCopies);
  const [refused, setRefused] = useState<
    Readonly<Record<AddressLine, readonly string[]>>
  >({ to: [], cc: [], bcc: [] });
  return (
    <LocaleProvider initial={locale}>
      <AddressBlock
        to={to}
        onToChange={setTo}
        cc={cc}
        onCcChange={setCc}
        bcc={bcc}
        onBccChange={setBcc}
        copiesOpen={copiesOpen}
        onOpenCopies={() => setCopiesOpen(true)}
        suggestions={{ contacts: CONTACTS, colleagues: COLLEAGUES }}
        onToEditing={() => {}}
        refused={refused}
        onRefuse={(line, parts) =>
          setRefused((prev) => ({ ...prev, [line]: parts }))
        }
        invalidTo={false}
        needTo="Name at least one recipient."
        deadRecipients={[]}
      />
      <output>{`to=${to.join(",")} cc=${cc.join(",")}`}</output>
    </LocaleProvider>
  );
}

const line = (name: string) =>
  screen.getByRole<HTMLInputElement>("combobox", { name });
const held = () => screen.getByRole("status").textContent;

describe("an address already on the message", () => {
  it("is refused in its own line in other capitals, and stays typed", async () => {
    const user = userEvent.setup();
    render(<Head start={["buyer1@brandt.example"]} />);
    await user.type(line("To"), "BUYER1@BRANDT.EXAMPLE{Enter}");
    expect(held()).toBe("to=buyer1@brandt.example cc=");
    expect(line("To").value).toBe("BUYER1@BRANDT.EXAMPLE");
    expect(screen.getByText("Already in To")).toBeTruthy();
  });

  it("is refused in another line, naming the line that holds it", async () => {
    const user = userEvent.setup();
    render(<Head start={["buyer1@brandt.example"]} openCopies />);
    await user.type(line("Cc"), "Buyer1@Brandt.example{Enter}");
    expect(held()).toBe("to=buyer1@brandt.example cc=");
    expect(line("Cc").value).toBe("Buyer1@Brandt.example");
    expect(screen.getByText("Already in To")).toBeTruthy();
  });

  it("leaves no refusal once the typed text is cleared", async () => {
    const user = userEvent.setup();
    render(<Head start={["buyer1@brandt.example"]} openCopies />);
    await user.type(line("Cc"), "BUYER1@BRANDT.EXAMPLE{Enter}");
    expect(screen.getByText("Already in To")).toBeTruthy();
    await user.keyboard("{Control>}a{/Control}{Backspace}");
    expect(line("Cc").value).toBe("");
    expect(screen.queryByText("Already in To")).toBeNull();
  });

  it("leaves no refusal once the typed text is deleted key by key", async () => {
    const user = userEvent.setup();
    render(<Head start={["buyer1@brandt.example"]} />);
    await user.type(line("To"), "BUYER1@BRANDT.EXAMPLE{Enter}");
    await user.keyboard("{Backspace>21/}");
    expect(line("To").value).toBe("");
    expect(screen.queryByText("Already in To")).toBeNull();
    expect(held()).toBe("to=buyer1@brandt.example cc=");
  });

  it("is no longer offered in any line", async () => {
    const user = userEvent.setup();
    render(<Head start={["buyer1@brandt.example"]} openCopies />);
    await user.click(line("Bcc"));
    expect(screen.queryByRole("option", { name: /Buyer 1 Brandt/ })).toBeNull();
  });
});

describe("the offered recipients", () => {
  it("are at most eight of the record's contacts while nothing is typed", async () => {
    const user = userEvent.setup();
    render(<Head />);
    await user.click(line("To"));
    const options = screen.getAllByRole("option");
    expect(options).toHaveLength(8);
    expect(screen.queryByRole("option", { name: /Sofia/ })).toBeNull();
  });

  it("add colleagues under their heading once two letters are typed", async () => {
    const user = userEvent.setup();
    render(<Head />);
    await user.type(line("To"), "e");
    expect(screen.queryByRole("group", { name: "Colleagues" })).toBeNull();
    await user.keyboard("l");
    const group = screen.getByRole("group", { name: "Colleagues" });
    expect(
      within(group).getByRole("option", { name: /Sofia Keller/ }),
    ).toBeTruthy();
  });

  it("close once one is picked with Enter", async () => {
    const user = userEvent.setup();
    render(<Head />);
    await user.click(line("To"));
    await user.keyboard("{ArrowDown}{Enter}");
    expect(held()).toBe("to=buyer1@brandt.example cc=");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("close on Escape and leave the typed text", async () => {
    const user = userEvent.setup();
    render(<Head />);
    await user.type(line("To"), "buy");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(line("To").value).toBe("buy");
  });
});

describe("the blind copy", () => {
  it("says the other recipients do not see its addresses", () => {
    render(<Head openCopies />);
    expect(line("Bcc").getAttribute("aria-describedby")).toBeTruthy();
    expect(
      screen.getByText("Other recipients do not see these addresses."),
    ).toBeTruthy();
  });
});

describe("a recipient's remove button", () => {
  it("is named in the reader's language", () => {
    render(<Head start={["stranger@example.com"]} locale="de" />);
    expect(
      screen.getByRole("button", { name: "stranger@example.com entfernen" }),
    ).toBeTruthy();
  });
});
