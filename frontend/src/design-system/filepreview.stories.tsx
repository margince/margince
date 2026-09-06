// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { FileChip, previewMediaType } from "./filechip";
import { FilePreviewProvider, useFilePreview } from "./filepreview";

// A stored file, opened over the page it was clicked on: the header band with
// the three verbs a reader of a document has, and the browser's own viewer
// under it. The chip that opens it is behind the scrim, which is the point —
// the record is still what the reader is in the middle of.

const meta: Meta = {
  title: "Design System/FilePreview",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

// A scan of a signed page, small enough to live in this file. Data rather than
// a fixture on a server so the READY state is a real one — the preview fetches
// this exactly as it fetches an attachment, and the frame draws what came back.
const SCAN =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAeAAAAEsCAIAAACUnPcNAAADiklEQVR42u3YsQ1CMRBEwS/KcHKBG6ABaqcAWsCBE9dBBejSFcxoK3DwdPJVj7uZmQXudgEQSaABBBoAgQYQaAAEGkCgARBoAAQaQKABEGgAgQZAoAEQaACBBkCgAQQaAIEGEGgABBoAgQYQaAAEGkCgARBoAAQaQKABEGgAgQZAoAEEGgCBBkCgAQQaAIEGEGgABBoAgQYQaAAEGkCgARBoAIH2BAACDYBAAwg0AAININAACDQAAg0g0AAINIBAAyDQAAg0gEADINAAAg2AQAMINAA5gX4/X2ZmFjgXNIAvDgAEGkCgARBoAIEGQKABEGgAgQZAoAEEGgCBBkCgAQQaAIEGEGgABBpAoAEQaAAEGkCgARBoAIEGQKABEGgAgQZAoAEEGgCBBhBoAAQaAIEGEGgABBpAoAEQaAAEGkCgARBoAIEGQKABBNoTAAg0AAININAACDSAQAMg0AAINIBAAyDQAAINgEADINAAAg2AQAMINAACDfCXgV5nm5lZ4FzQAL44ABBoAIEGQKABBBoAgQZAoAEEGgCBBhBoAAQaAIEGEGgABBpAoAEQaACBBkCgAegCPUeZmVngXNAAvjgAEGgAgQZAoAEEGgCBBkCgAQQaAIEGEGgABBoAgQYQaAAEGkCgARBoAIEGQKAB6AK9zjYzs8C5oAF8cQAg0AACDYBAAwg0AAINgEADCDQAAg0g0AAINAACDSDQAAg0gEADINAAAg2AQAPQBXqOMjOzwLmgAXxxACDQAAINgEADCDQAAg2AQAMINAACDSDQAAg0AAININAACDSAQAMg0AACDYBAA9AFep1tZmaBc0ED+OIAQKABBBoAgQYQaAAEGgCBBhBoAAQaQKABEGgABBpAoAEQaACBBkCgAQQaAIEGoAv0HGVmZoFzQQP44gBAoAEEGgCBBhBoAAQaAIEGEGgABBpAoAEQaAAEGkCgARBoAIEGQKABBBoAgQagC/Q628zMAueCBvDFAYBAAwg0AAININAACDQAAg0g0AAINMBPB3qOMjOzwLmgAXxxACDQAAINgEADCDQAAg2AQAMINAACDSDQAAg0AAININAACDSAQAMg0AACDYBAAyDQAAINgEADCDQAAg2AQAMINAACDSDQAAg0gEADINAACDSAQAMg0AACDYBAAyDQAAINgEADCDQAAg0g0J4AQKABEGgAgQZAoAEEGgCBBuCbD2ZGCbsODbxeAAAAAElFTkSuQmCC";

// A story is read at a glance, so the dialog is already open rather than one
// click away. It opens through the same call the chip makes, with the same
// media type the chip would have decided from the filename.
function OpenOnMount({
  href,
  filename,
}: Readonly<{ href: string; filename: string }>) {
  const preview = useFilePreview();
  const mediaType = previewMediaType(filename);
  useEffect(() => {
    if (preview !== null && mediaType !== null) {
      preview.open({ href, filename, mediaType });
    }
  }, [preview, mediaType, href, filename]);
  return null;
}

function Opened({
  href,
  filename,
}: Readonly<{ href: string; filename: string }>) {
  return (
    <FilePreviewProvider>
      <div style={{ padding: "var(--space-4)" }}>
        <FileChip href={href} filename={filename} size="248 kB" />
      </div>
      <OpenOnMount href={href} filename={filename} />
    </FilePreviewProvider>
  );
}

export const Opens: Story = {
  render: () => <Opened href={SCAN} filename="GR-2026-0092-signed.png" />,
};

// What a file the store could not answer for looks like. The reader keeps the
// one move left to them — Download is beside the sentence, in the band — and
// is told nothing about why the read failed, which is not theirs to see.
//
// The refusal is a real one: the story asks for a file nothing serves, and the
// browser says so in the console on its way to the state below. That console
// line is the SUBJECT here rather than a fault, so the story carries the tag
// that says so, the way a story about an error boundary does.
export const CouldNotBeShown: Story = {
  tags: ["uat-expected-console-error"],
  render: () => (
    <Opened href="/v1/attachments/gone" filename="framework-agreement.pdf" />
  ),
};

// A file nothing here can draw. The card stays what it always was: the click
// saves it, and no dialog opens over a reader who is about to open it in the
// application that owns the format.
export const StaysADownload: Story = {
  render: () => (
    <FilePreviewProvider>
      <div style={{ padding: "var(--space-4)" }}>
        <FileChip
          href="/v1/attachments/a-2"
          filename="terms-redline.docx"
          size="61 kB"
        />
      </div>
    </FilePreviewProvider>
  ),
};
