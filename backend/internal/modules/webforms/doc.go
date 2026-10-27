// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package webforms turns a website's form into leads. A web_form row is one
// public intake address, POST /v1/public/forms/{form_key}. Its sender says
// who may post: a browser from an allowed website, a server holding the form's
// secret, or a builder's signed webhook.
//
// The module keeps no submission. An admitted post becomes a lead with a note
// on its timeline, written through the LeadLander seam compose injects. An
// email a lead or contact already holds gets the note on that record. So the
// visitor's data lives where erasure and retention already reach it.
//
// Tables owned: docs/reference/entity-model/webforms.md.
package webforms
