// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var formDefinitions = map[string]Definition{
	"form.subject":        {Words: langcopy.Phrase{En: "Website form: %s", De: "Website-Formular: %s", Vi: "Biểu mẫu website: %s"}, Arguments: []string{argumentText}},
	"form.consent_no":     {Words: langcopy.Phrase{En: "Consent: not ticked", De: "Einwilligung: nicht angekreuzt", Vi: "Đồng ý: chưa đánh dấu"}, Arguments: []string{}},
	"form.consent_yes":    {Words: langcopy.Phrase{En: "Consent: ticked: %s", De: "Einwilligung: angekreuzt: %s", Vi: "Đồng ý: đã đánh dấu: %s"}, Arguments: []string{argumentText}},
	"form.consent_policy": {Words: langcopy.Phrase{En: "Consent: ticked: %s (policy %s)", De: "Einwilligung: angekreuzt: %s (Richtlinie %s)", Vi: "Đồng ý: đã đánh dấu: %s (chính sách %s)"}, Arguments: []string{argumentText, argumentText}},
	"core.source":         {Words: langcopy.Phrase{En: "%s", De: "%s", Vi: "%s"}, Arguments: []string{argumentText}},
}
