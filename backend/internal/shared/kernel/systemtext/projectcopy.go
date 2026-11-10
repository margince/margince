// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var projectDefinitions = map[string]Definition{
	"project.quiet":           {Words: langcopy.Phrase{En: "Nothing has been filed on this project for at least %d days.", De: "Zu diesem Projekt wurde seit mindestens %d Tagen nichts abgelegt.", Vi: "Không có gì được lưu cho dự án này trong ít nhất %d ngày."}, Arguments: []string{argumentCount}},
	"project.overdue":         {Words: langcopy.Phrase{En: "An open task on this project is past its due date.", De: "Eine offene Aufgabe in diesem Projekt ist überfällig.", Vi: "Một công việc đang mở của dự án này đã quá hạn."}, Arguments: []string{}},
	"project.quietAndOverdue": {Words: langcopy.Phrase{En: "Nothing has been filed on this project for at least %d days, and an open task on it is past its due date.", De: "Zu diesem Projekt wurde seit mindestens %d Tagen nichts abgelegt, und eine offene Aufgabe darin ist überfällig.", Vi: "Không có gì được lưu cho dự án này trong ít nhất %d ngày, và một công việc đang mở của dự án đã quá hạn."}, Arguments: []string{argumentCount}},
	"project.active":          {Words: langcopy.Phrase{En: "Activity is being filed on this project and no open task is past its due date.", De: "Zu diesem Projekt wird Aktivität abgelegt, und keine offene Aufgabe ist überfällig.", Vi: "Dự án này vẫn có hoạt động được lưu và không có công việc đang mở nào quá hạn."}, Arguments: []string{}},
	"project.noticeSubject":   {Words: langcopy.Phrase{En: "%s is now %s", De: "%s ist jetzt %s", Vi: "%s hiện %s"}, Arguments: []string{argumentText, argumentText}},
	"project.noticeUndo":      {Words: langcopy.Phrase{En: "The AI set this from the project's activity. Open the project to undo it.", De: "Die KI hat das aus der Aktivität des Projekts abgeleitet. Öffne das Projekt, um es rückgängig zu machen.", Vi: "AI đã đặt tình trạng này dựa trên hoạt động của dự án. Mở dự án để hoàn tác."}, Arguments: []string{}},
	"project.onTrack":         {Words: langcopy.Phrase{En: "on track", De: "im Plan", Vi: "đúng kế hoạch"}, Arguments: []string{}},
	"project.atRisk":          {Words: langcopy.Phrase{En: "at risk", De: "gefährdet", Vi: "có rủi ro"}, Arguments: []string{}},
	"project.offTrack":        {Words: langcopy.Phrase{En: "off track", De: "außer Plan", Vi: "chệch kế hoạch"}, Arguments: []string{}},
}
