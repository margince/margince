// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

var voiceFailureWords = struct{ Failed, Quota, Unconfigured, Busy, Refused, Withheld, Rejected, Unavailable, Invalid, WordFloor langcopy.Phrase }{
	Failed:       langcopy.Phrase{En: "The build failed. Try again.", De: "Die Erstellung ist fehlgeschlagen. Versuche es erneut.", Vi: "Tạo hồ sơ thất bại. Hãy thử lại."},
	Quota:        langcopy.Phrase{En: "Our AI provider refused the call: the account behind it is out of budget or over its quota, so the build never ran. Your previous version is unchanged. Raise the limit on the provider account, then build again.", De: "Der KI-Anbieter hat den Aufruf wegen eines ausgeschöpften Budgets oder Kontingents abgelehnt. Die Erstellung wurde nicht ausgeführt; die bisherige Version bleibt unverändert. Erhöhe das Limit beim Anbieter und starte erneut.", Vi: "Nhà cung cấp AI từ chối yêu cầu vì tài khoản đã hết ngân sách hoặc vượt hạn mức, nên quá trình tạo chưa chạy. Phiên bản trước không thay đổi. Tăng hạn mức ở tài khoản nhà cung cấp rồi tạo lại."},
	Unconfigured: langcopy.Phrase{En: "Voice building is unavailable until an AI provider is configured: the offline stand-in model answered instead. Open Settings → AI, bind each tier to a vendor and add that vendor's key under AI models → Providers, then build again.", De: "Die Stilerstellung ist erst verfügbar, wenn ein KI-Anbieter eingerichtet ist. Stattdessen hat das Offline-Ersatzmodell geantwortet. Ordne unter Einstellungen → KI jeder Stufe einen Anbieter zu und hinterlege dessen Schlüssel unter KI-Modelle → Anbieter. Starte danach erneut.", Vi: "Chỉ có thể tạo phong cách viết khi đã cấu hình nhà cung cấp AI: mô hình thay thế ngoại tuyến đã trả lời. Mở Cài đặt → AI, gán nhà cung cấp cho từng cấp và thêm khóa của họ trong Mô hình AI → Nhà cung cấp, rồi tạo lại."},
	Busy:         langcopy.Phrase{En: "Our AI provider is busy right now and turned the call away, so the build did not run. Your previous version is unchanged; wait a minute and build again.", De: "Der KI-Anbieter ist gerade ausgelastet und hat den Aufruf abgelehnt. Die Erstellung wurde nicht ausgeführt; die bisherige Version bleibt unverändert. Warte eine Minute und starte erneut.", Vi: "Nhà cung cấp AI hiện đang bận và đã từ chối yêu cầu, nên quá trình tạo chưa chạy. Phiên bản trước không thay đổi; đợi một phút rồi tạo lại."},
	Refused:      langcopy.Phrase{En: "Our AI provider turned the call away, so the build never ran. Your previous version is unchanged. Check the provider's status and account, then build again.", De: "Der KI-Anbieter hat den Aufruf abgelehnt. Die Erstellung wurde nicht ausgeführt; die bisherige Version bleibt unverändert. Prüfe Status und Konto beim Anbieter und starte erneut.", Vi: "Nhà cung cấp AI đã từ chối yêu cầu, nên quá trình tạo chưa chạy. Phiên bản trước không thay đổi. Kiểm tra trạng thái và tài khoản nhà cung cấp rồi tạo lại."},
	Withheld:     langcopy.Phrase{En: "The AI model declined to write this voice profile, so the build produced nothing. Your previous version is unchanged. Building again may meet the same answer; a model bound to another vendor under Settings → AI may not.", De: "Das KI-Modell hat die Erstellung dieses Stilprofils abgelehnt. Die bisherige Version bleibt unverändert. Ein erneuter Versuch kann dieselbe Antwort ergeben; ein anderer Anbieter unter Einstellungen → KI möglicherweise nicht.", Vi: "Mô hình AI từ chối tạo hồ sơ phong cách này, nên không tạo được gì. Phiên bản trước không thay đổi. Thử lại có thể nhận cùng câu trả lời; mô hình từ nhà cung cấp khác trong Cài đặt → AI có thể cho kết quả khác."},
	Rejected:     langcopy.Phrase{En: "Our AI provider rejected the build request as malformed, so the build never ran. Your previous version is unchanged. This is a fault on our side rather than in your samples; the server log carries the provider's reason.", De: "Der KI-Anbieter hat die Anfrage als fehlerhaft abgelehnt. Die Erstellung wurde nicht ausgeführt; die bisherige Version bleibt unverändert. Der Fehler liegt bei uns, nicht an deinen Schreibproben. Das Serverprotokoll enthält die Begründung des Anbieters.", Vi: "Nhà cung cấp AI từ chối yêu cầu tạo vì định dạng sai, nên quá trình tạo chưa chạy. Phiên bản trước không thay đổi. Đây là lỗi phía chúng tôi, không phải ở mẫu viết của bạn; nhật ký máy chủ có lý do từ nhà cung cấp."},
	Unavailable:  langcopy.Phrase{En: "Voice building is unavailable until an AI provider is configured.", De: "Die Stilerstellung ist erst verfügbar, wenn ein KI-Anbieter eingerichtet ist.", Vi: "Chỉ có thể tạo phong cách viết khi đã cấu hình nhà cung cấp AI."},
	Invalid:      langcopy.Phrase{En: "The voice model returned something we could not read as a profile. Your previous version is unchanged; try again, and if it keeps failing the model or its configuration is at fault rather than your samples.", De: "Die Antwort des Stilmodells konnte nicht als Profil gelesen werden. Die bisherige Version bleibt unverändert. Versuche es erneut. Bei wiederholten Fehlern liegt es am Modell oder seiner Konfiguration, nicht an deinen Schreibproben.", Vi: "Mô hình phong cách trả về nội dung không thể đọc thành hồ sơ. Phiên bản trước không thay đổi; hãy thử lại. Nếu lỗi tiếp diễn, nguyên nhân nằm ở mô hình hoặc cấu hình, không phải mẫu viết của bạn."}, //nolint:misspell // German translation uses German spelling.
	WordFloor:    langcopy.Phrase{En: "voice build needs at least %d own-authored words; corpus has %d", De: "Die Stilerstellung benötigt mindestens %d selbst geschriebene Wörter; die Sammlung enthält %d", Vi: "Tạo phong cách cần ít nhất %d từ do bạn tự viết; tập mẫu có %d"},
}

// SafeVoiceBuildFailure keeps provider internals out of the recorded explanation.
func SafeVoiceBuildFailure(err error, language textlang.Lang) string {
	if err == nil {
		return voiceFailureWords.Failed.In(language)
	}
	if errors.Is(err, ErrProviderQuota) {
		return voiceFailureWords.Quota.In(language)
	}
	if errors.Is(err, ErrUnconfiguredModel) {
		return voiceFailureWords.Unconfigured.In(language)
	}
	if errors.Is(err, ErrProviderThrottled) {
		return voiceFailureWords.Busy.In(language)
	}
	if errors.Is(err, errProviderRefused) {
		return voiceFailureWords.Refused.In(language)
	}
	if errors.Is(err, model.ErrOutputWithheld) {
		return voiceFailureWords.Withheld.In(language)
	}
	if errors.Is(err, model.ErrRequestRejected) {
		return voiceFailureWords.Rejected.In(language)
	}
	if floor, ok := errors.AsType[*voiceWordFloorError](err); ok {
		return fmt.Sprintf(voiceFailureWords.WordFloor.In(language), StarterVoiceWords, floor.words)
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "no model path"), strings.Contains(text, "no bound"),
		strings.Contains(text, "not bound"), strings.Contains(text, "unbound"):
		return VoiceBuildUnavailable(language)
	default:
		return voiceFailureWords.Invalid.In(language)
	}
}

// VoiceBuildUnavailable explains the missing model binding in the writer's language.
func VoiceBuildUnavailable(language textlang.Lang) string {
	return voiceFailureWords.Unavailable.In(language)
}

type voiceWordFloorError struct{ words int }

func (e *voiceWordFloorError) Error() string {
	return fmt.Sprintf(voiceFailureWords.WordFloor.En, StarterVoiceWords, e.words)
}
