// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type documentCopy struct{ langcopy.Spoken }

func documentCopyFor(language string) documentCopy { return documentCopy{langcopy.For(language)} }

var documentWords = struct{ NoFields, Bytes, NoType, UnsupportedType, PDFNoText, PDFUnreadable, Chars, Empty, Mislabelled, Refused, Missing, NoStorage, NoModel langcopy.Phrase }{
	NoFields:        langcopy.Phrase{En: "this document states none of the deal fields clearly enough to offer one", De: "Dieses Dokument nennt keines der Deal-Felder eindeutig genug für einen Vorschlag.", Vi: "Tài liệu này không nêu rõ trường deal nào để đề xuất."},
	Bytes:           langcopy.Phrase{En: "this document is larger than the %d MB one reading carries; a reading of part of it could not say which part it saw", De: "Dieses Dokument überschreitet die Grenze von %d MB pro Lesevorgang. Beim Lesen eines Ausschnitts wäre nicht klar, welcher Teil erfasst wurde.", Vi: "Tài liệu vượt giới hạn %d MB mỗi lần đọc; đọc một phần sẽ không xác định được phần nào đã được xem."},
	NoType:          langcopy.Phrase{En: "this document declares no content type, so nothing can say how to read it", De: "Für dieses Dokument ist kein Inhaltstyp angegeben. Daher ist unklar, wie es gelesen werden kann.", Vi: "Tài liệu không có loại nội dung nên không xác định được cách đọc."},
	UnsupportedType: langcopy.Phrase{En: "this installation's model cannot read a %s document; a file whose text can be read directly, or a model bound to carry documents, would be read", De: "Das konfigurierte Modell kann ein Dokument vom Typ %s nicht lesen. Verwende eine direkt lesbare Textdatei oder ein Modell, das Dokumente unterstützt.", Vi: "Mô hình đã cấu hình không đọc được tài liệu loại %s. Hãy dùng tệp văn bản đọc trực tiếp được hoặc mô hình hỗ trợ tài liệu."},
	PDFNoText:       langcopy.Phrase{En: "this PDF holds no text, which is what a scanned or photographed document looks like; a PDF exported from the system that produced it can be read", De: "Dieses PDF enthält keinen Text, wie bei einem Scan oder Foto. Ein aus dem Ursprungssystem exportiertes PDF kann gelesen werden.", Vi: "PDF này không chứa văn bản, như tài liệu quét hoặc chụp ảnh. Có thể đọc bản PDF xuất từ hệ thống tạo tài liệu."},
	PDFUnreadable:   langcopy.Phrase{En: "this PDF could not be opened to read its text", De: "Dieses PDF konnte zum Lesen des Textes nicht geöffnet werden.", Vi: "Không mở được PDF này để đọc văn bản."},
	Chars:           langcopy.Phrase{En: "this document is %d characters, and one reading addresses at most %d", De: "Dieses Dokument enthält %d Zeichen. Ein Lesevorgang erfasst höchstens %d.", Vi: "Tài liệu có %d ký tự; mỗi lần đọc xử lý tối đa %d."},
	Empty:           langcopy.Phrase{En: "this document carries no text to read", De: "Dieses Dokument enthält keinen lesbaren Text.", Vi: "Tài liệu không có văn bản để đọc."},
	Mislabelled:     langcopy.Phrase{En: "this document's bytes are not the type it was uploaded as, so no model can read it; upload it again as the file it actually is", De: "Der Dateiinhalt entspricht nicht dem beim Hochladen angegebenen Typ. Lade die Datei erneut mit ihrem tatsächlichen Typ hoch.", Vi: "Nội dung tệp không khớp với loại đã khai báo khi tải lên nên mô hình không đọc được. Hãy tải lại tệp với đúng loại."},
	Refused:         langcopy.Phrase{En: "the model's reading of this document could not be used; the document is unchanged and can be read again", De: "Das Leseergebnis des Modells konnte nicht verwendet werden. Das Dokument ist unverändert und kann erneut gelesen werden.", Vi: "Không sử dụng được kết quả đọc của mô hình. Tài liệu không thay đổi và có thể đọc lại."},
	Missing:         langcopy.Phrase{En: "this document is no longer available to read", De: "Dieses Dokument ist nicht mehr zum Lesen verfügbar.", Vi: "Tài liệu không còn khả dụng để đọc."},
	NoStorage:       langcopy.Phrase{En: "this installation stores no document bytes, so there is nothing to read", De: "Es werden keine Dateiinhalte gespeichert. Daher steht kein Dokument zum Lesen bereit.", Vi: "Hệ thống không lưu nội dung tệp nên không có tài liệu để đọc."},
	NoModel:         langcopy.Phrase{En: "this installation has no AI model configured for reading documents", De: "Es ist kein KI-Modell zum Lesen von Dokumenten konfiguriert.", Vi: "Chưa cấu hình mô hình AI để đọc tài liệu."},
}
