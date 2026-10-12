// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "../screens/common";
import { Button, Field, Modal, TextInput } from "./atoms";
import { ErrorLine } from "./errorline";
import { Heading } from "./heading";
import { Row, Stack } from "./stack";

// The one refusal line, in each of the shapes a caller hands it. Flip the
// Theme control: the ink is `--dangerText`, which is lifted in dark.
const meta: Meta = {
  title: "Components/Messaging/Error line",
  component: ErrorLine,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

const conflict = new ProblemError({
  code: "version_conflict",
  detail: "Somebody else changed this record. Re-read it and try again.",
});

export const MutationFailure: Story = {
  name: "A mutation's error",
  render: () => <ErrorLine error={conflict} />,
};

export const Sentence: Story = {
  name: "A sentence the caller translated",
  render: () => (
    <ErrorLine>This message has more than one recipient.</ErrorLine>
  ),
};

export const WithActions: Story = {
  name: "With a verb on the line",
  render: () => (
    <ErrorLine
      error={conflict}
      actions={
        <Button variant="ghost" onClick={() => undefined}>
          Re-read
        </Button>
      }
    />
  ),
};

export const DescribesAControl: Story = {
  name: "Describing the control above it",
  render: () => (
    <Stack gap="2">
      <TextInput
        aria-label="VAT number"
        aria-describedby="vat-refusal"
        aria-invalid
        defaultValue="DE12"
      />
      <ErrorLine id="vat-refusal">
        A VAT number has nine digits after the country code.
      </ErrorLine>
    </Stack>
  ),
};

export const Inline: Story = {
  name: "Inline, in its control's row",
  render: () => (
    <Row gap="2">
      <TextInput
        aria-label="Discount"
        aria-describedby="discount-refusal"
        aria-invalid
        defaultValue="140"
      />
      <ErrorLine inline id="discount-refusal">
        A discount cannot exceed 100%.
      </ErrorLine>
    </Row>
  ),
};

export const Standing: Story = {
  name: "Standing: true when the surface drew, not announced",
  render: () => (
    <ErrorLine standing>
      You can read this room, but only its owner can post in it.
    </ErrorLine>
  ),
};

export const NothingToReport: Story = {
  name: "No error: draws nothing",
  render: () => <ErrorLine error={null} />,
};

const mailboxFields = [
  "Anzeigename",
  "E-Mail-Adresse",
  "Antwortadresse",
  "Signatur",
  "Posteingang",
  "Gesendete Elemente",
  "Archiv",
  "Abgleich ab",
  "Benutzername",
  "App-Passwort",
  "IMAP-Server",
  "Port",
];

const settingsAfter = ["SMTP-Server", "SMTP-Port", "Verschlüsselung"];

function PhoneSheetRefusal() {
  const titleId = useId();
  const [refusal, setRefusal] = useState<ProblemError | null>(null);
  const refuse = () =>
    setRefusal(
      new ProblemError({
        code: "imap_unreachable",
        detail: "Der Server antwortet nicht. Prüfen Sie Host und Port.",
      }),
    );
  return (
    <Modal open onClose={() => undefined} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        Postfach verbinden
      </Heading>
      <div className="form-stack">
        {mailboxFields.map((label) => (
          <Field key={label} label={label}>
            {(control) => <TextInput {...control} />}
          </Field>
        ))}
        <ErrorLine error={refusal} />
        {settingsAfter.map((label) => (
          <Field key={label} label={label}>
            {(control) => <TextInput {...control} />}
          </Field>
        ))}
      </div>
      <div className="actions">
        <Button>Später noch einmal versuchen</Button>
        <Button variant="primary" onClick={refuse}>
          Postfach jetzt verbinden
        </Button>
      </div>
    </Modal>
  );
}

/** A refusal arriving below the fold of a phone sheet stops above its action
 * row, however many lines the row wraps to. */
export const InAPhoneSheet: Story = {
  name: "In a phone sheet whose actions wrap",
  render: () => <PhoneSheetRefusal />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    const dialog = await screen.findByRole("dialog", {
      name: "Postfach verbinden",
    });
    const actions = dialog.querySelector<HTMLElement>(":scope > .actions");
    await expect(actions && getComputedStyle(actions).position).toBe("sticky");
    const press = within(dialog).getByRole("button", {
      name: "Postfach jetzt verbinden",
    });
    const oneRow = press.getBoundingClientRect().height;
    await expect(actions?.getBoundingClientRect().height).toBeGreaterThan(
      oneRow * 2,
    );
    await userEvent.click(press);
    const alert = await within(dialog).findByRole("alert");
    await waitFor(
      () =>
        expect(alert.getBoundingClientRect().bottom).toBeLessThanOrEqual(
          (actions?.getBoundingClientRect().top ?? 0) + 1,
        ),
      { timeout: 3000 },
    );
  },
};
