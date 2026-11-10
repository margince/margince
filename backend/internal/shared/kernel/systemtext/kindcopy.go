// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var kindDefinitions = map[string]Definition{
	"activity_kind.message":       {Words: langcopy.Phrase{En: argumentMessage, De: "Nachricht", Vi: "tin nhắn"}},
	"activity_kind.email":         {Words: langcopy.Phrase{En: "email", De: "E-Mail", Vi: "email"}},
	"activity_kind.meeting":       {Words: langcopy.Phrase{En: "meeting", De: "Besprechung", Vi: "cuộc họp"}},
	"activity_kind.call":          {Words: langcopy.Phrase{En: "call", De: "Anruf", Vi: "cuộc gọi"}},
	"activity_kind.note":          {Words: langcopy.Phrase{En: "note", De: "Notiz", Vi: "ghi chú"}},
	"activity_kind.task":          {Words: langcopy.Phrase{En: "task", De: "Aufgabe", Vi: "việc cần làm"}},
	"action_kind.create_record":   {Words: langcopy.Phrase{En: "create_record", De: "einen Datensatz anlegen", Vi: "tạo bản ghi"}},
	"action_kind.update_record":   {Words: langcopy.Phrase{En: "update_record", De: "einen Datensatz ändern", Vi: "cập nhật bản ghi"}},
	"action_kind.create_task":     {Words: langcopy.Phrase{En: "create_task", De: "eine Aufgabe anlegen", Vi: "tạo việc cần làm"}},
	"action_kind.assign_owner":    {Words: langcopy.Phrase{En: "assign_owner", De: "einen Inhaber zuweisen", Vi: "gán chủ sở hữu"}},
	"action_kind.advance_deal":    {Words: langcopy.Phrase{En: "advance_deal", De: "einen Deal weiterführen", Vi: "chuyển giai đoạn deal"}},
	"action_kind.send_email":      {Words: langcopy.Phrase{En: "send_email", De: "eine E-Mail senden", Vi: "gửi email"}},
	"action_kind.emit_flow_event": {Words: langcopy.Phrase{En: "emit_flow_event", De: "einen Ablauf auslösen", Vi: "kích hoạt luồng"}},
	"action_kind.recompute_score": {Words: langcopy.Phrase{En: "recompute_score", De: "eine Bewertung neu berechnen", Vi: "tính lại điểm"}},
	"action_kind.enqueue_job":     {Words: langcopy.Phrase{En: "enqueue_job", De: "einen Auftrag einplanen", Vi: "lên lịch tác vụ"}},
	"action_kind.notify":          {Words: langcopy.Phrase{En: "notify", De: "eine Nachricht senden", Vi: "gửi thông báo"}},
	"action_kind.draft_email":     {Words: langcopy.Phrase{En: "draft_email", De: "eine E-Mail entwerfen", Vi: "soạn email"}},
	"action_kind.add_list_member": {Words: langcopy.Phrase{En: "add_list_member", De: "einen Listeneintrag hinzufügen", Vi: "thêm mục vào danh sách"}},
}

// ActivityKindArgument localizes known activity kinds and preserves unknown wire values.
func ActivityKindArgument(kind string) Argument { return kindArgument("activity_kind.", kind) }

// ActionKindArgument localizes known action kinds and preserves unknown wire values.
func ActionKindArgument(kind string) Argument { return kindArgument("action_kind.", kind) }

func kindArgument(prefix, kind string) Argument {
	if _, known := definitionOf(prefix + kind); known {
		return MessageArgument(prefix + kind)
	}
	return Text(kind)
}
