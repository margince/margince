# UI copy: Vietnamese

Vietnamese is translated from the English catalog beside it: `frontend/src/i18n/en.ts` for
`frontend/src/i18n/vi.ts`, and the sibling `en.json` for each extension's `vi.json` (for example
`extensions/openchannel/frontend/i18n/en.json`). It also follows [ui-copy-style.md](ui-copy-style.md); this
page states only what Vietnamese adds or changes. [ui-copy-style-de.md](ui-copy-style-de.md) is a sibling, not
a source: where German keeps a loanword, Vietnamese decides.

The target is Vietnamese business software: a formal written register, the reader's world instead of the
system's internals, and fewer pronouns than the English.

Part of it is mechanical. `frontend/src/i18n/copy-style-vi.test.ts` holds the mechanics, the voice, the
address and the retired words over `vi.ts` and every extension `vi.json`; `frontend/src/i18n/record-noun.test.ts`
holds the record noun; `frontend/src/i18n/i18n.test.ts` holds key and placeholder parity and the tenant word.
[What the gate holds](#what-the-gate-holds) lists them. Everything else is the author's judgement, and a
reviewer reads a Vietnamese change against this page.

## Source and claim strength

- **The English value is the source of meaning.** When the English and the current Vietnamese disagree about
  a fact, the English wins; when Vietnamese grammar and English syntax disagree, Vietnamese wins. Translate
  what a value means and does, not its words.
- **Nothing added, nothing dropped.** No fact, reassurance, feature or caveat the English does not carry. A
  label stays a label: "Permissions" is "Quyền", never "Họ được làm gì?".
- **Claim strength is kept.** Legal, privacy, consent, retention, licence and AI-notice text keeps every
  qualifier. That covers can and may (có thể), designed to (được thiết kế để), intended to (nhằm) and
  subject to (tùy thuộc vào). It also covers where applicable (trong trường hợp áp dụng), typically (thường)
  and does not guarantee (không đảm bảo). "Designed to support compliance" never becomes "đảm bảo tuân thủ".
- **No superlative the English does not state.** "nhất", "duy nhất", "hàng đầu" and "số 1" are regulated
  claims under Luật Quảng cáo 16/2012/QH13.
- **Unchanged**: Margince, Gradion, Voice DNA, Deal Room, vendor and product names (Google Workspace,
  Microsoft 365, OpenRouter), URLs, code, file extensions, identifiers and placeholders.
- **A vendor's label** quoted in “…” stays as the English quotes it: the language of the reader's vendor
  console is unknown.

## Address

- **Address nobody by default.** Labels, titles, statuses, table headers, menu items, toasts and most hints
  address nobody.
- **Fewer pronouns than the English.** Write bạn or của bạn only where the English says you or your, and
  then only where it is the one way to say whose, or who must act. Drop it where context names the owner:
  "từ khi mở danh sách", "Tin nhắn vẫn nằm ngoài danh sách". Keep it in "Bạn không có quyền thực hiện thao
  tác này" and beside "Deal của nhóm". Lowercase mid-sentence; a capital only where a sentence opens.
- **Five families are read by an outsider**: the four German Sie families in
  `frontend/src/i18n/address-register.test.ts` (`privacynotice.`, `confirm.`, `buyer.`, `prefs.`) and
  `book.`, the public booking form a guest fills in. There the reader is **quý khách** where the English
  says you, never bạn, and "Vui lòng" is allowed. Use quý khách only where the sentence needs it: "Xin cảm
  ơn", "Xác nhận đăng ký". `book.name` and `book.email` share a form with `scheduling.` keys, so they
  address nobody ("Họ và tên", "Email").
- **Never** quý vị, anh/chị, anh chị or kính thưa; no "Vui lòng" outside those families; no "nhé", "nha" or
  "ạ"; no "xin" except "Xin cảm ơn" on an outsider page; no mixed address on one surface.
- **A contact who acts** is người liên hệ or người này; the stored record stays liên hệ. "Người liên hệ chỉ
  đọc phần này"; "Gửi email cho người này một liên kết riêng". Such a key joins the Vietnamese human-sense
  list in `record-noun.test.ts`.
- **Five consent keys follow the server**: `confirm.marketing.*` and `confirm.subscription.*` equal the
  server copy in `backend/internal/platform/mailcopy/catalog.go` byte for byte, and a consent proof records
  that wording. They change there first, with `marketingQuestionVersion` bumped; the gate skips them.

## Never tôi, chúng tôi, chúng ta or mình

Margince is software and never speaks as itself: "Chúng tôi không lưu được"
becomes "Không thể lưu thay đổi". The exceptions are the English page's, and
the gate reads them from `copy-style.test.ts`:

| Where | May say | Why |
|---|---|---|
| `ob.conv.` | tôi where the agent acts ("để tôi dựng lại"); a noun phrase where one works ("Nội dung đã đọc") | the onboarding agent speaks in a chat bubble, in short factual sentences |
| `privacynotice.` | chúng tôi, as little as the sentence allows, never a bare tôi | the data controller speaks, as in law |
| `prefs.wording.`, `prefs.wordingGeneric`, `directSend.acknowledge`, `book.consentWording` | tôi, chúng tôi, chúng ta; "của mình" instead of a second "của tôi" | a consent statement is the reader's own sentence |
| a value whose English says me, my or mine | tôi | "Giao cho tôi", "Chỉ tôi", "Deal của tôi" name the reader in a control |

A reader's answer on a consent page may speak as the reader: "Không gửi tin cho tôi".
Reflexive mình refers back to a third party: "của mình", "chính mình", "riêng
mình", "tự mình", "một mình", "chỉ mình". A bare "tên mình" reads as "my name",
so write "tên của mình". Status lines of the trợ lý AI are neutral ("Đang tóm tắt tuần…",
never "Tôi đang…"), and "waiting on us" follows the English: "Đang chờ nhóm của
bạn".

## Register

| Situation | Write | Not |
|---|---|---|
| An error | Không thể hoàn tác; Không thể tải tệp; khách không thể đặt lịch qua đó nữa | Không hoàn tác được; không đặt lịch được nữa |
| A result line | Tải xuống không thành công | Không bắt đầu tải xuống được |
| Something adverse happened | bị lỗi, bị gián đoạn, bị hủy, bị xóa | thất bại in a title or status line; dừng giữa chừng |
| Work running long | Quá trình phân tích {name} kéo dài bất thường | Phân tích {name} đang lâu bất thường |
| A status line: aspect, then verb | Đang chờ đọc trang web công ty; Đã đọc xong trang web {name}; Đang phân tích… | Lượt đọc trang web đang chờ xử lý; Lượt đọc trang web {name} đã hoàn tất |
| A counter of discrete events | {count} lượt gọi; lượt xem; Lần dựng này for one run | lượt as the subject of a status line |
| Asking an admin | Liên hệ quản trị viên; Liên hệ quản trị viên để được cấp thêm quyền | Nhờ quản trị viên; quản trị viên hệ thống |
| A failure the reader cannot fix | Mã kết nối AI không hợp lệ; Đã hết hạn mức sử dụng AI; Không rõ nguyên nhân | Nhà cung cấp AI từ chối thông tin xác thực đã cấu hình; Hệ thống không báo nguyên nhân |

- **bị is allowed for an adverse outcome.** The ban on the passive "được … bởi" stays: there the
  system or an agent does something ordinary, and Vietnamese names no actor.
- **Outcome over mechanism.** Say what happened to the reader and what to do next. An admin-only
  settings screen, where the admin acts on the provider, may still name the provider and the credential.
- **Simplifying never drops a fact** the English states: a limit, a consequence, who can act.

## Grammar by slot

| Slot | Shape | Example |
|---|---|---|
| Button, menu item | verb or verb and object, no "Hãy", no period | Lưu thay đổi, Tạo deal, Thử lại |
| Label, heading, tab, table header | noun phrase, no period; "các" only before a definite set | Ngày chốt, Deal đang mở, Các khía cạnh đã đánh giá |
| Status, badge | short adjective, "Đã" and verb, or "bị" and an adverse outcome | Đã gửi, Quá hạn, Chờ duyệt, Bị lỗi |
| In progress | "Đang" and verb, one ellipsis character | Đang lưu…, Đang tải deal… |
| Success | "Đã", verb and object, no period | Đã thêm giai đoạn |
| Error | what did not happen, then the one action | Không thể lưu thay đổi. Kiểm tra kết nối rồi thử lại. |
| Warning | what will happen, then what it costs | Xóa giai đoạn này sẽ chuyển 12 deal về giai đoạn trước. |
| Confirmation | title asks and names the object; primary repeats verb and object, never Có, OK or a bare Xác nhận; secondary is Hủy | Xóa giai đoạn? / Xóa giai đoạn / Hủy |
| Empty state | one fact, one action | Chưa có deal nào. Tạo deal đầu tiên. |
| Permission denied | the fact and who can change it | Chỉ quản trị viên sửa được hạn mức. |
| Withheld | what is missing and why | Số tiền bị ẩn với vai trò này. |
| Hint, tooltip | one sentence on what the field takes | Số nguyên, từ 1 đến 1.000.000. |
| Input placeholder | example or noun list, the English ellipsis | Tìm liên hệ, công ty, deal… |

- An error says "Không thể" and verb ("Không thể tải trường ngày. Thử lại."), never "Đã có lỗi xảy
  ra" alone, and never blames the reader.
- An action sentence opens with its verb; "Hãy" only where a bare verb would
  read like a label.
- The system as actor stays implicit ("Không thể lưu", "Đã lưu"). An agent acts with "do": "Do {agent}
  tự động làm", never "bởi {agent}".

## Mechanics

- **Tone marks kiểu cũ**: on the first vowel of an open oa, oe or uy (hòa, xóa, khóa, tùy, hủy, khỏe).
  Before a final consonant it goes on the second, in both styles (hoàn, khuyên). "quý" and "quỹ" are qu and
  y.
- **NFC only**: an NFD value looks identical and breaks search and length.
- **Sentence case**: a capital on the first word and on proper nouns only, never "Cài Đặt Tài Khoản". A
  named screen keeps its nav capital mid-sentence ("trong Cài đặt"). deal, lead and pipeline are lowercase
  mid-sentence.
- **No dashes**: no em dash, en dash, "--" or spaced hyphen. Use a comma, a period, a colon or parentheses.
  A range is "{start} đến {end}".
- **Quotes** are “…”; the apostrophe is ’. Never a straight " or ', never „, » or «.
- **Ellipsis** is the one character …, only where the English has one. End punctuation and edge spaces
  mirror the English.
- **No !, no double space**, and no space before ? : … or a comma.
- **No abbreviations**: v.v., vd., v/v and & are written out ("ví dụ", "và") or cut. Established acronyms
  stay: CRM, API, AI, VAT, PDF, CSV, UTC, DNS, IMAP, GDPR.
- **Numbers, money, dates and times** come from the formatting layer through a placeholder, never grouped or
  spelled by hand. A range is "1 đến 4"; percent sits on the number ("{pct}%"); relative time is "vừa xong",
  "hôm qua".
- **Placeholders** keep the English set and move wherever Vietnamese order wants them; "Nhãn: {count}" where
  a noun reads wrong at 0.
- **No plural.** Vietnamese has one form, and `Intl.PluralRules` gives `other` for every count. `X_one`
  equals `X_other`. Where the English arms carry different placeholders, `_one` carries the same
  placeholders as the English `_one` and otherwise reads like `_other`. Never "các" or "những" in front of
  every plural.

## Fluency

| Rule | Write | Not |
|---|---|---|
| các or những where the English plural is definite | Các lệnh ở trên vẫn hoạt động; Các việc còn lại ở bên dưới | Lệnh ở trên vẫn hoạt động |
| sẽ for a consequence or a future effect | Báo giá sẽ dùng loại tiền của deal; Quyền sẽ bị hủy ở yêu cầu tiếp theo | Báo giá dùng tiền tệ của deal |
| Chưa for a state that can still change, Không for a fact | Chưa phản hồi; chưa có liên lạc gần đây | Không có phản hồi |
| An all-clear in the affirmative | Mọi việc đã thống nhất đều đang được tiến hành. | Không có việc đã thống nhất nào đang chững lại. |
| Topic first; a compressed English noun phrase opens into a clause | các mục đích còn lại do quý khách lựa chọn; cần thiết cho các việc quý khách đã yêu cầu | quý khách tự quyết định mọi mục đích còn lại; cần cho yêu cầu của quý khách |
| Idiom over a literal rendering | mức thường thấy, vượt mức, lời mời trên lịch | giá trị thông thường, lời mời lịch |

## Natural Vietnamese, not calques

| Write | Not | The trap |
|---|---|---|
| Không thể lưu thay đổi | Việc lưu thay đổi thất bại, Sự lưu thay đổi | việc and sự turn every verb into a noun |
| Đổi tên; Người đổi: {actor} | Thực hiện đổi tên; Người thực hiện | thực hiện and tiến hành pad a verb that needs no help |
| nhanh, rõ ràng | một cách nhanh chóng, một cách rõ ràng | một cách mirrors an English -ly adverb |
| Do {agent} tự động làm; {name} đã nhập | Được thực hiện bởi {agent}; Được nhập bởi {name} | được and bởi copy the English passive |
| Đã lưu | Đã được lưu | được where the system acts |
| Lưu thay đổi | Lưu các thay đổi của bạn | của bạn and các on a control the reader already owns |
| Ngày chốt dự kiến của deal | Deal dự kiến chốt ngày | English noun chains kept in English order |
| Tải lại trang | Làm tươi trang, Làm mới lại trang | refresh, word by word |
| Đã có lỗi xảy ra. Thử lại. | Cái gì đó đã đi sai | something went wrong, word by word |
| Chưa có mục nào | Trạng thái trống | a design term shown as a message |
| Gỡ quan hệ này? | Bạn chắc chứ? | a question about the reader instead of the object |
| (nothing) | Xin lưu ý, Chỉ cần, Đơn giản là, Cứ thoải mái | padding the English does not say |

## Which English stays English

- **Loanwords** deal, lead and pipeline take a classifier, never an English
  plural ("3 deal", "một lead").
- **Product nouns** stay: Commit, Best case, passport, token, webhook,
  endpoint, email, API, CRM, AI, MCP, OAuth, VAT, GDPR, Deal Room, Voice DNA.
  The word agent stays only where the English names an external agent client or a protocol object, such as an
  agent connection or a passport scope, and in identifiers.
- **Everything else is Vietnamese**, even what an engineer keeps in English:
  máy chủ, tệp, trang web, tải lên, báo cáo, việc cần làm, thẻ, cuộc họp.

## Length

Vietnamese runs 25 to 30% longer than English. Controls, chips and headers
take the shortest natural phrasing: cut "các", "những", "của bạn", "thông tin",
"việc", "thực hiện". The English ceilings apply: 3 English words on a button
or label (about five syllables), 6 on a title, one sentence of about 110
characters for a hint, two sentences of about 220 for a message. Legal text is
exempt. Meaning is never truncated to fit; a surface that clips correct
Vietnamese is a layout bug.

## Vocabulary

One Vietnamese word per concept, the same word on every screen. In the Never
column, `copy-style-vi.test.ts` retires a word in code format. Such a word fails
as a whole word in any case, and an English one in its plural too, outside a
quoted vendor label, a file name and a URL. A word goes in code format only
when no value has a legitimate use for it; one that does stays plain, held by
the reviewer in the sense its row gives.

| Concept | Vietnamese | Never |
|---|---|---|
| The tenant: the reader's own company, its settings, its allowance | công ty (Hồ sơ công ty, cả công ty); tổ chức only for a body that is not the tenant, or a vendor's own label | không gian làm việc and workspace (i18n.test.ts holds both); tổ chức for the tenant |
| A company record: customer, prospect, partner | công ty | tổ chức or tài khoản for a company record; the retired company nouns in [record-vocabulary.md](record-vocabulary.md) |
| A record as a stored thing (contact, company, deal, lead, project) | hồ sơ (Chi tiết hồ sơ, Đang tải hồ sơ…); an audit entry or an import row is mục or dòng | bản ghi for a record (bản ghi DNS keeps it) |
| The legal entity behind an installation | pháp nhân | `thực thể pháp lý`; tổ chức for the legal entity |
| A human record | liên hệ; the contact as someone who acts is người liên hệ or người này | `đầu mối`; người for the record, and the retired record nouns in [record-vocabulary.md](record-vocabulary.md) |
| Sales object | deal | `thương vụ`; giao dịch or cơ hội for a deal (email giao dịch, tên giao dịch and the company lifecycle stage keep theirs) |
| Lead | lead | khách hàng tiềm năng or khách tiềm năng for a lead (the Prospect lifecycle stage keeps it) |
| Lead handling (settings) | Quản lý lead | xử lý lead for the settings surface |
| Ordered stages | pipeline | `phễu`; quy trình bán hàng for the pipeline |
| Commercial motion; sales motion | hình thức bán hàng (deal field); cách bán hàng (company profile) | |
| A pipeline step | giai đoạn | chặng for a step (chặng đường is ordinary Vietnamese) |
| Forecast object | dự báo | `dự phóng`, `forecast` |
| Forecast categories | Commit, Best case | `trường hợp tốt nhất`, `khả quan nhất`; Cam kết for the category |
| Forecast category (the field) | danh mục dự báo | nhóm dự báo (nhóm is a team) |
| Manager's forecast number | nhận định (Nhận định của quản lý) | đánh giá or dự báo của quản lý for this number |
| Something a party said they would do | cam kết | `lời hứa` |
| The reader's queue | Danh sách công việc; a control inside the Worklist screen itself may say Danh sách; an item in it is việc | `worklist`, `hàng đợi công việc`; hộp thư đến for the Worklist; mục for a Worklist item |
| Daily digest | Bản tin sáng | `bản tóm tắt buổi sáng`, `briefing` |
| Approvals surface; approve | Phê duyệt; duyệt, đã duyệt | `chấp thuận` |
| Review (look again before acting) | xem lại; weekly review tổng kết tuần; outcome review đánh giá | rà soát |
| Snooze | tạm hoãn (Not now / set aside = gác lại, Để sau) | |
| A classifier's result | kết quả, lần kiểm tra | `phán quyết`, `verdict` |
| Capture intake step | kiểm tra đầu vào | `admission check` |
| Ownership of a deal | phụ trách (label Phụ trách; the human người phụ trách; Assign owner = Giao người phụ trách) | người sở hữu for ownership |
| Tabs of a record | the tab labels themselves | `phần của bản ghi` |
| A pointer to a Settings page | Cài đặt, mục followed by the tab or group label as the screen shows it | an arrow between Cài đặt and the label; lowercase cài đặt before the label |
| The running Margince system | bản cài đặt | |
| Administrator; member; users; staff | quản trị viên (Liên hệ quản trị viên); thành viên; người dùng; nhân viên, nhân viên kinh doanh | `quản trị viên hệ thống`, `nhờ quản trị viên` |
| Operations role holder | thành viên Vận hành (role.ops = Vận hành) | |
| Member status | đang hoạt động, đã mời, đã vô hiệu hóa (deactivated), đã tạm khóa (suspended) | tạm ngưng for a suspended member |
| Agent credential | passport | `hộ chiếu`; token or khóa for a passport (khóa API and khóa ký name other credentials) |
| Mail or calendar link | trình kết nối | `bộ kết nối`, `connector`; kết nối alone or tích hợp for a connector |
| Buyer-facing deal page | Deal Room | `phòng deal`; cổng thông tin for the Deal Room |
| The mail thread of a record | chuỗi thư; a record spine's mixed-channel thread is chuỗi trao đổi; a Deal Room discussion thread is chủ đề | `spine` |
| Import of mailbox history | nhập lịch sử hộp thư | `backread`; đọc ngược for the import |
| Full read of a web page | đọc toàn trang | `đọc sâu`, `deep read` |
| The AI agent | trợ lý AI in a title or tab and wherever an agent is the actor (history, audit, notifications, drafts, the license count); trợ lý once the context is set; agent only where a screen configures an external agent client: MCP connections, passports and their scopes, an approval kind that names a passport | `tác tử`; agent for the product's own assistant or for an actor |
| Artificial intelligence | AI | `TTNT` |
| Sign in, sign out | đăng nhập, đăng xuất | `login`, `log in`, `đăng nhập vào trong` |
| Retry | Thử lại | `thử lần nữa` |
| Upload, download | tải lên, tải xuống | `upload`, `download` |
| Enter a value; show | Nhập; Hiển thị | Đặt for entering a value (đặt lịch keeps it); trình bày for show (it means present) |
| Recommended (a badge) | Khuyên dùng | nên dùng for the badge |
| Stopped partway; failed | bị gián đoạn; bị lỗi, or không thành công for a result; a neutral Stopped status is Đã dừng, an adverse one Bị dừng | `dừng giữa chừng`; thất bại in a title, status or counter |
| Email | email | `thư điện tử`, `e-mail` |
| Message | tin nhắn; mail inside a mailbox thư; one email message email | |
| Mailbox | hộp thư | `mailbox` |
| Meeting; its attendee | cuộc họp; người tham dự | `buổi họp`, `meeting`; khách for an attendee (khách is a booking guest) |
| Task | việc cần làm where the word stands alone; việc where the task list sets the context (Tạo việc, việc quá hạn); a background job or a kind of AI work (Tác vụ AI) is tác vụ | `task`; nhiệm vụ or công việc for a task |
| Tag | thẻ | `tag`; nhãn for a tag |
| Shortlist; Live List | danh sách chọn; danh sách động, lowercase mid-sentence (Picklist field type = Danh sách lựa chọn) | |
| Report | báo cáo | `report` |
| AI quota | hạn mức | `định mức` |
| Credit | credit (enrichment units); số dư (an AI provider's prepaid balance, on an admin screen) | |
| Currency | loại tiền; a custom field of type Currency is Số tiền | tiền tệ in UI copy |
| Licensed seat | tài khoản (tài khoản đầy đủ quyền, tài khoản chỉ xem); a seat holder is thành viên | `chỗ ngồi`, `ghế`, `suất` (hiệu suất, xác suất, tần suất and thuế suất keep theirs) |
| Retention; legal hold | lưu trữ (thời hạn lưu trữ, quy tắc lưu trữ); lưu trữ pháp lý | `legal hold`, `lưu giữ pháp lý`; nghĩa vụ lưu trữ for the hold (a statutory retention obligation keeps it) |
| Archive | chuyển vào kho lưu trữ; state trong kho lưu trữ; unarchive khôi phục từ kho lưu trữ (Khôi phục beside the archived item) | bare lưu trữ for archive (it is retention) |
| Route into a buyer | hướng tiếp cận | |
| Quiet relationship; drifting deal | im lặng; đang chững lại (going cold = đang nguội dần) | |
| Stalled deal | đình trệ (Deal bị đình trệ in a sentence) | đang chững lại for stalled (that is drifting) |
| Buying committee, buying center | nhóm quyết định mua | `buying center`, `nhóm mua hàng` |
| Audit log | Nhật ký hoạt động | `nhật ký kiểm tra`, `nhật ký kiểm toán`, `audit log`, `audit trail` |
| Knowledge (settings, sources) | Tài liệu; a knowledge base is kho tài liệu | kiến thức for the settings surface |
| Follow-up | việc tiếp theo; the verb follow up is liên hệ lại | `follow-up`; theo dõi for a follow-up (it is watch) |
| Priced offer, quote | báo giá | `chào giá` |
| Close date; a closed deal, won or lost; close as an outcome | ngày chốt (ngày chốt dự kiến); đã đóng; kết thúc | `ngày đóng`; chốt for a deal closed won or lost (chốt is won: Doanh số đã chốt, Sắp chốt) |
| File | tệp | `file` |
| Website | trang web | `website` |
| Server | máy chủ | `server` |
| Reason | lý do | `lí do` |
| Evidence | bằng chứng | `chứng cứ` |
| Transcript | bản chép lời | `bản ghi lời` |
| Enrichment | bổ sung dữ liệu | `làm giàu dữ liệu` |
| Open channel endpoint | endpoint | `điểm cuối` |
| Consent (the legal concept) | sự đồng ý (verb: đồng ý) | |
| Capture of mail and meetings | thu thập; a report snapshot is chụp | |
| Remove, delete | gỡ, xóa | |
| Writing voice, its profile, its corpus; signing secret; champion | giọng văn, hồ sơ giọng văn, kho văn mẫu, a writing sample văn mẫu; khóa ký; người ủng hộ | mẫu văn for a writing sample |

**người** appears only where a human being is meant, and then the key is on the
Vietnamese human-sense list in `record-noun.test.ts`. Prefer thành viên, quản
trị viên or ai where they read naturally.

## Worked pairs from the native review

| Rule | Before | Native |
|---|---|---|
| Record | Chi tiết bản ghi | Chi tiết hồ sơ |
| Tenant | Hồ sơ tổ chức | Hồ sơ công ty |
| AI agent | Agent | Trợ lý AI |
| Seat | Suất và giấy phép | Tài khoản và giấy phép |
| Retention | Quyền riêng tư và lưu giữ | Quyền riêng tư và lưu trữ |
| Audit log | Nhật ký kiểm tra | Nhật ký hoạt động |
| Error, pronoun | Không hoàn tác được. Tin nhắn vẫn nằm ngoài danh sách của bạn. | Không thể hoàn tác. Tin nhắn vẫn nằm ngoài danh sách. |
| Adverse outcome | Hành động đã duyệt thất bại | Hành động đã duyệt bị lỗi |
| Adverse outcome | Bản tin sáng dừng giữa chừng | Bản tin sáng bị gián đoạn |
| Running long | Tóm tắt {name} đang lâu bất thường | Quá trình tóm tắt {name} kéo dài bất thường |
| Status line | Lượt đọc trang web công ty đang chờ xử lý | Đang chờ đọc trang web công ty |
| Status line | Lượt đọc trang web {name} đã hoàn tất | Đã đọc xong trang web {name} |
| Asking an admin | Suất này chỉ đọc, nên yêu cầu bị từ chối. Nhờ quản trị viên nâng cấp suất. | Tài khoản này chỉ có quyền xem. Liên hệ quản trị viên để nâng cấp tài khoản. |
| Outcome over mechanism | Nhà cung cấp AI từ chối thông tin xác thực đã cấu hình. Liên hệ quản trị viên hệ thống. | Mã kết nối AI không hợp lệ. Liên hệ quản trị viên. |
| Contact who acts | Liên hệ chỉ đọc phần này. | Người liên hệ chỉ đọc phần này. |
| Outsider address | Cảm ơn quý khách | Xin cảm ơn |
| Controller voice | Phản đối cách chúng tôi sử dụng thông tin đó. | Phản đối cách sử dụng thông tin. |
| Agent voice | Những gì tôi đã đọc | Nội dung đã đọc |
| các, sẽ, Nhập, loại tiền | Đặt giá trị deal trước. Báo giá dùng tiền tệ của deal. | Nhập giá trị deal trước. Báo giá sẽ dùng loại tiền của deal. |
| Chưa for a state | Không có phản hồi | Chưa phản hồi |

## Contested choices

- **công ty** names the tenant and a company record: the English says company for both, and context
  separates them, as it separates hồ sơ the record from hồ sơ giọng văn.
- **Kiểu cũ**: the product owner's call. Both styles are correct; mixing is not.
- **Danh sách công việc**, **Bản tin sáng**, **nhận định**: the loanwords are deal, lead and pipeline only;
  đánh giá is a rating, dự báo the forecast.
- **Commit, Best case, passport, endpoint stay English**: cam kết is the commitment, "tốt nhất" a regulated
  superlative and hộ chiếu a travel document.
- **trợ lý AI**, **tài khoản**, **Nhật ký hoạt động**: what Vietnamese business software shows. Chỗ ngồi is
  a chair, and kiểm toán is a financial audit.
- **trình kết nối**, **việc cần làm**, **thẻ**: Google's and Microsoft's Vietnamese. A UI card is "khung",
  so thẻ stays unambiguous.
- **lưu trữ is retention**, so archive takes kho lưu trữ. **nhóm quyết định mua** and **việc tiếp theo**
  have no settled loanword, and theo dõi is watch, which the product also does.
- **máy chủ, tệp, trang web**: the reader is a sales team, not engineers; thu thập is the verb of Nghị định
  13/2023/NĐ-CP on data protection.

## Before you write Vietnamese

1. Read this page, then the English value and where it renders, before the
   current Vietnamese: the old Vietnamese drifted from older English.
2. Look every noun up in the Vocabulary table. A missing concept gets one
   word, added to the table in the same change.
3. A Vietnamese technical-writing skill helps with calques and code-switching,
   if your harness has one; this page wins where they disagree.
4. From `frontend/`, run `pnpm exec vitest run src/i18n/` and fix every value
   the gate names.

## What the gate holds

`frontend/src/i18n/copy-style-vi.test.ts` checks every value of `vi.ts` and of
every extension `vi.json`, one test per rule, and lists each offender as its
source file, key and value. Placeholders are removed before the word rules run,
and the five server-worded consent keys are skipped.

| Rule | What fails |
|---|---|
| NFC | A value that NFC normalization changes |
| Tone marks | A mark on the second vowel of an open oa, oe or uy (hoá, tuỳ); a mark on the first vowel before a final consonant |
| No dashes | An en dash, an em dash, `--` or a spaced hyphen |
| Ellipsis | Three dots in a row |
| No exclamation marks | Any `!` |
| Quotes | A straight `"`, „, » or «; a straight `'` between two letters |
| Spacing | Two spaces in a row |
| Abbreviations | v.v., vd., v/v or &, outside a quoted vendor label |
| One plural form | An `_one` value that differs from its `_other`, where the English arms carry the same placeholders |
| Never tôi | tôi outside `ob.conv.`, the consent statements and a value whose English says me, my or mine |
| Never chúng tôi | chúng tôi outside `privacynotice.` and the consent statements |
| Never chúng ta or mình | Either outside the consent statements; reflexive mình is removed first |
| Formal address | quý vị, anh/chị, anh chị or kính thưa anywhere |
| Outsider address | quý khách or vui lòng outside the outsider families; bạn inside them |
| Capitals | Bạn or Quý khách capitalised where no sentence opens |
| Retired vocabulary | A word in code format in the Vocabulary table's Never column; suất only outside the compounds its row keeps |
| No failure word in a short value | thất bại in a value of at most 8 words: a title, label, status or counter |

The prefixes come from the gates that own them: the speaker exemptions from
`copy-style.test.ts`, the outsider families from `address-register.test.ts`
with `book.` added, the consent keys from `marketingquestion_test.go`. A last
test fails any difference between the Never column's code-format words and the
words the test retires.

`record-noun.test.ts` holds the record noun: a record-type key that does not
say liên hệ, and người outside its Vietnamese human-sense list. `i18n.test.ts`
holds key and placeholder parity, untranslated values and the tenant word.
`backend/gates/uiautonomyclaims_test.go` holds claims that nothing is ever sent
without approval.

None of them sees meaning and claim strength, calques, bạn where nobody needed
addressing, or mixed address on one surface. Nor do they see the register (Không thể, bị, a status line
led by its verb), slot grammar, end punctuation and edge spaces against the English, sentence case,
hand-formatted numbers, length, or the plain words of the Never column. Those are the author's and the
reviewer's judgement.
