// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var healthWords = struct {
	Stage, NoActivity, DaysAgo, Today, Yesterday, Stalled, Active langcopy.Phrase
	NoEngagement, OneEngagement, ManyEngaged                      langcopy.Phrase
	NoOverdue, OneOverdue, ManyOverdue                            langcopy.Phrase
}{
	Stage:         langcopy.Phrase{En: "Days in this stage: %d; won deals: about %d.", De: "Tage in dieser Phase: %d; bei gewonnenen Deals etwa %d.", Vi: "%d ngày ở giai đoạn này; giao dịch thành công mất khoảng %d ngày."},
	NoActivity:    langcopy.Phrase{En: "No activity has been logged on this deal.", De: "Für diesen Deal wurde noch keine Aktivität erfasst.", Vi: "Chưa có hoạt động nào được ghi nhận cho giao dịch này."},
	DaysAgo:       langcopy.Phrase{En: "%d days ago", De: "vor %d Tagen", Vi: "%d ngày trước"},
	Today:         langcopy.Phrase{En: "today", De: "heute", Vi: "hôm nay"},
	Yesterday:     langcopy.Phrase{En: "yesterday", De: "gestern", Vi: "hôm qua"},
	Stalled:       langcopy.Phrase{En: "Last activity %s — the deal counts as stalled.", De: "Letzte Aktivität %s – der Deal gilt als festgefahren.", Vi: "Hoạt động gần nhất: %s – giao dịch được coi là đình trệ."},
	Active:        langcopy.Phrase{En: "Last activity %s.", De: "Letzte Aktivität %s.", Vi: "Hoạt động gần nhất: %s."},
	NoEngagement:  langcopy.Phrase{En: "No stakeholder has been in two-way contact in the last %d days.", De: "In den letzten %d Tagen gab es mit keinem Beteiligten einen beidseitigen Austausch.", Vi: "Không có bên liên quan nào trao đổi hai chiều trong %d ngày qua."},
	OneEngagement: langcopy.Phrase{En: "One stakeholder in two-way contact in the last %d days.", De: "In den letzten %d Tagen gab es mit einem Beteiligten einen beidseitigen Austausch.", Vi: "Một bên liên quan đã trao đổi hai chiều trong %d ngày qua."},
	ManyEngaged:   langcopy.Phrase{En: "%d stakeholders in two-way contact in the last %d days.", De: "Mit %d Beteiligten gab es in den letzten %d Tagen einen beidseitigen Austausch.", Vi: "%d bên liên quan đã trao đổi hai chiều trong %d ngày qua."},
	NoOverdue:     langcopy.Phrase{En: "No overdue task on this deal.", De: "Keine überfällige Aufgabe für diesen Deal.", Vi: "Không có nhiệm vụ quá hạn cho giao dịch này."},
	OneOverdue:    langcopy.Phrase{En: "One task is overdue.", De: "Eine Aufgabe ist überfällig.", Vi: "Một nhiệm vụ đã quá hạn."},
	ManyOverdue:   langcopy.Phrase{En: "%d tasks are overdue.", De: "%d Aufgaben sind überfällig.", Vi: "%d nhiệm vụ đã quá hạn."},
}
