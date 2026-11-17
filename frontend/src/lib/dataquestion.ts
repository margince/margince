// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This selects the reader, never a query or a value; the server plans the question.
export function looksLikeDataQuestion(question: string): boolean {
  return /\b(how many|how much|(?:which|what) (?:deals|contacts|companies|leads|projects|tasks)|open pipeline|sales won|total|sum|count|won|lost|wie viele|wie viel|welche (?:deals|kontakte|unternehmen|leads|projekte)|gewonnen|verloren|bao nhiêu|tổng|doanh thu)\b/iu.test(
    question,
  );
}
