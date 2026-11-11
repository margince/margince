// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { HOME_DAY_FRESH_MS, isHomeDayRead, worklistKey } from "../lib/workkeys";
import { type Answer, type HomeRead, readerIdFrom } from "./homejourney.judge";

export type RetainedAnswer = { read: HomeRead; journey: string; ageMs: number };

export class AnswerHistory {
  private document = 0;
  private trusted: boolean;
  private answers = new Map<HomeRead, { answer: Answer; at: number }>();

  constructor(
    private readonly reader: string | undefined,
    private readonly now: () => number,
  ) {
    this.trusted = Boolean(reader);
  }

  beginDocument(): void {
    this.document++;
    this.answers.clear();
    this.trusted = Boolean(this.reader);
  }

  documentID(): number {
    return this.document;
  }

  received(answer: Answer, document: number): void {
    if (document !== this.document) return;
    if (
      answer.read === "me" &&
      (answer.status !== 200 || readerIdFrom(answer.body) !== this.reader)
    ) {
      this.forgetReader();
      return;
    }
    if (!this.trusted) return;
    if (answer.status !== 200 || answer.body === undefined) {
      this.answers.delete(answer.read);
      return;
    }
    this.answers.set(answer.read, { answer, at: this.now() });
  }

  failed(read: HomeRead, document: number): void {
    if (document !== this.document) return;
    if (read === "me") this.forgetReader();
    else this.answers.delete(read);
  }

  private forgetReader(): void {
    this.answers.clear();
    this.trusted = false;
  }

  fill(answers: readonly Answer[], requested: readonly HomeRead[]) {
    const combined = [...answers];
    const retained: RetainedAnswer[] = [];
    if (!this.trusted) return { answers: combined, retained };
    for (const [read, stored] of this.answers) {
      const key =
        read === "worklist" ? [...worklistKey, "mine", "all", ""] : [read];
      const ageMs = this.now() - stored.at;
      if (
        !isHomeDayRead(key) ||
        requested.includes(read) ||
        answers.some((answer) => answer.read === read) ||
        !Number.isFinite(ageMs) ||
        ageMs < 0 ||
        ageMs >= HOME_DAY_FRESH_MS
      )
        continue;
      combined.push(stored.answer);
      retained.push({ read, journey: stored.answer.journey, ageMs });
    }
    return { answers: combined, retained };
  }
}
