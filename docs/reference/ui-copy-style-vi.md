# UI copy: Vietnamese

Vietnamese is translated from the English catalog beside it: `frontend/src/i18n/en.ts`
for `frontend/src/i18n/vi.ts`, and the sibling `en.json` for each extension's
`vi.json` (for example `extensions/openchannel/frontend/i18n/en.json`).
[ui-copy-style.md](ui-copy-style.md) binds it too; this page states only what
Vietnamese adds or changes. [ui-copy-style-de.md](ui-copy-style-de.md) is a
sibling, not a source: where German keeps a loanword, Vietnamese decides.

Part of it is mechanical. `frontend/src/i18n/copy-style-vi.test.ts` holds the
mechanics, the voice, the address and the retired words over `vi.ts` and every
extension `vi.json`; `frontend/src/i18n/record-noun.test.ts` holds the record
noun; `frontend/src/i18n/i18n.test.ts` holds key and placeholder parity and the
tenant word. [What the gate holds](#what-the-gate-holds) lists them. Everything
else is the author's judgement, and a reviewer reads a Vietnamese change
against this page.

## Source and claim strength

- **The English value is the source of meaning.** When the English and the
  current Vietnamese disagree about a fact, the English wins; when Vietnamese
  grammar and English syntax disagree, Vietnamese wins. Translate what a value
  means and does, not its words.
- **Nothing added, nothing dropped.** No fact, reassurance, feature or caveat
  the English does not carry. A label stays a label: "Permissions" is "Quyền",
  never "Họ được làm gì?".
- **Claim strength is kept exactly.** Legal, privacy, consent, retention,
  licence and AI-notice text keeps every qualifier: can and may (có thể),
  designed to (được thiết kế để), intended to (nhằm), subject to (tùy thuộc
  vào), where applicable (trong trường hợp áp dụng), typically (thường), does
  not guarantee (không đảm bảo). "Designed to support compliance" never becomes
  "đảm bảo tuân thủ".
- **No superlative the English does not state.** "nhất", "duy nhất", "hàng
  đầu" and "số 1" are regulated claims under Luật Quảng cáo 16/2012/QH13.
- **Unchanged**: Margince, Gradion, Voice DNA, Deal Room, vendor and product
  names (Google Workspace, Microsoft 365, OpenRouter), URLs, code, file
  extensions, identifiers and placeholders.
- **A vendor's label** quoted in “…” stays exactly as the English quotes it:
  the language of the reader's vendor console is unknown.

## Address

- **Address nobody by default.** Labels, titles, statuses, table headers, menu
  items, toasts and most hints address nobody.
- **bạn only where the English says you or your**, and then only when the
  sentence needs a subject or an owner: "Deal của bạn" beside "Deal của nhóm",
  but "Đăng nhập lại để tiếp tục" needs none. Lowercase mid-sentence; a capital
  only where a sentence opens.
- **Five families are read by an outsider**: the four German Sie families in
  `frontend/src/i18n/address-register.test.ts` (`privacynotice.`, `confirm.`,
  `buyer.`, `prefs.`) and `book.`, the public booking form a guest fills in.
  There the reader is **quý khách** where the English says you, never bạn, and
  "Vui lòng" is allowed; still address nobody where the English allows it.
  `book.name` and `book.email` share a form with `scheduling.` keys, so they
  address nobody ("Họ và tên", "Email").
- **Never** quý vị, anh/chị, anh chị or kính thưa; no "Vui lòng" outside those
  families; no "xin", "nhé", "nha" or "ạ"; no mixed address on one surface.
- **Five consent keys follow the server**: `confirm.marketing.*` and
  `confirm.subscription.*` equal the server copy in
  `backend/internal/platform/mailcopy/catalog.go` byte for byte, and a consent
  proof records that wording. They change there first, with
  `marketingQuestionVersion` bumped; the gate skips them.

## Never tôi, chúng tôi, chúng ta or mình

Margince is software and never speaks as itself: "Chúng tôi không lưu được"
becomes "Không lưu được thay đổi". The exceptions are the English page's, and
the gate reads them from `copy-style.test.ts`:

| Where | May say | Why |
|---|---|---|
| `ob.conv.` | tôi | the onboarding agent speaks in a chat bubble, in short factual sentences |
| `privacynotice.` | chúng tôi, never a bare tôi | the data controller speaks, as in law |
| `prefs.wording.`, `prefs.wordingGeneric`, `directSend.acknowledge`, `book.consentWording` | tôi, chúng tôi, chúng ta | a consent statement is the reader's own sentence |
| a value whose English says me, my or mine | tôi | "Giao cho tôi", "Chỉ tôi", "Deal của tôi" name the reader in a control |

Reflexive mình refers back to a third party: "của mình", "chính mình", "riêng
mình", "tự mình", "một mình", "chỉ mình". A bare "tên mình" reads as "my name",
so write "tên của mình". Agent status lines are neutral ("Đang tóm tắt tuần…",
never "Tôi đang…"), and "waiting on us" follows the English: "Đang chờ nhóm của
bạn".

## Grammar by slot

| Slot | Shape | Example |
|---|---|---|
| Button, menu item | verb or verb and object, no "Hãy", no period | Lưu thay đổi, Tạo deal, Thử lại |
| Label, heading, tab, table header | noun phrase, no "các" or "những", no period | Ngày chốt, Deal đang mở |
| Status, badge | short adjective or "Đã" and verb | Đã gửi, Quá hạn, Chờ duyệt |
| In progress | "Đang" and verb, one ellipsis character | Đang lưu…, Đang tải deal… |
| Success | "Đã", verb and object, no period | Đã thêm giai đoạn |
| Error | what did not happen, then the one action | Không lưu được thay đổi. Kiểm tra kết nối rồi thử lại. |
| Warning | what will happen, then what it costs | Xóa giai đoạn này sẽ chuyển 12 deal về giai đoạn trước. |
| Confirmation | title asks and names the object; primary repeats verb and object, never Có, OK or a bare Xác nhận; secondary is Hủy | Xóa giai đoạn? / Xóa giai đoạn / Hủy |
| Empty state | one fact, one action | Chưa có deal nào. Tạo deal đầu tiên. |
| Permission denied | the fact and who can change it | Chỉ quản trị viên sửa được hạn mức. |
| Withheld | what is missing and why | Số tiền bị ẩn với vai trò này. |
| Hint, tooltip | one sentence on what the field takes | Số nguyên, từ 1 đến 1.000.000. |
| Input placeholder | example or noun list, the English ellipsis | Tìm liên hệ, công ty, deal… |

- An error says "Không" and verb and "được" ("Không tải được trường ngày. Thử
  lại."), never "Đã có lỗi xảy ra" alone, and never blames the reader.
- An action sentence opens with its verb; "Hãy" only where a bare verb would
  read like a label.
- The system as actor stays implicit ("Không lưu được", "Đã lưu"). "bị" only
  for something adverse ("bị từ chối"). An agent acts with "do": "Do {agent} tự
  động làm", never "bởi {agent}".

## Mechanics

- **Tone marks kiểu cũ**: on the first vowel of an open oa, oe or uy (hòa,
  xóa, khóa, tùy, hủy, khỏe); before a final consonant on the second, in both
  styles (hoàn, khuyên). "quý" and "quỹ" are qu and y.
- **NFC only**: an NFD value looks identical and breaks search and length.
- **Sentence case**: a capital on the first word and on proper nouns only,
  never "Cài Đặt Tài Khoản". A named screen keeps its nav capital mid-sentence
  ("trong Cài đặt"). deal, lead and pipeline are lowercase mid-sentence.
- **No dashes**: no em dash, en dash, "--" or spaced hyphen. Use a comma, a
  period, a colon or parentheses. A range is "{start} đến {end}".
- **Quotes** are “…”; the apostrophe is ’. Never a straight " or ', never „, »
  or «.
- **Ellipsis** is the one character …, only where the English has one. End
  punctuation and edge spaces mirror the English.
- **No !, no double space**, and no space before ? : … or a comma.
- **No abbreviations**: v.v., vd., v/v and & are written out ("ví dụ", "và")
  or cut. Established acronyms stay: CRM, API, AI, VAT, PDF, CSV, UTC, DNS,
  IMAP, GDPR.
- **Numbers, money, dates and times** come from the formatting layer through a
  placeholder, never grouped or spelled by hand. A range is "1 đến 4"; percent
  sits on the number ("{pct}%"); relative time is "vừa xong", "hôm qua".
- **Placeholders** keep the English set and move wherever Vietnamese order
  wants them; "Nhãn: {count}" where a noun reads wrong at 0.
- **No plural.** Vietnamese has one form, and `Intl.PluralRules` gives `other`
  for every count. `X_one` equals `X_other`. Where the English arms carry
  different placeholders, `_one` carries exactly the English `_one`
  placeholders and otherwise reads like `_other`. Never "các" or "những" in
  front of every plural.

## Natural Vietnamese, not calques

| Write | Not | The trap |
|---|---|---|
| Không lưu được thay đổi | Việc lưu thay đổi thất bại, Sự lưu thay đổi | việc and sự turn every verb into a noun |
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
- **Product nouns** stay: Commit, Best case, agent, passport, token, webhook,
  endpoint, email, API, CRM, AI, MCP, OAuth, VAT, GDPR, Deal Room, Voice DNA.
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
column a word in code format is retired by `copy-style-vi.test.ts`: it fails as
a whole word in any case, and an English one in its plural too, outside a
quoted vendor label, a file name and a URL. A word goes in code format only
when no value has a legitimate use for it; one that does stays plain, held by
the reviewer in the sense its row gives.

| Concept | Vietnamese | Never |
|---|---|---|
| The tenant: the reader's own company, its settings, its allowance | tổ chức | không gian làm việc and workspace (i18n.test.ts holds both) |
| A company record: customer, prospect, partner | công ty | tổ chức or tài khoản for a company record; the retired company nouns in [record-vocabulary.md](record-vocabulary.md) |
| The legal entity behind an installation | pháp nhân | `thực thể pháp lý`; tổ chức for the legal entity |
| A human record | liên hệ | `đầu mối`; người for the record, and the retired record nouns in [record-vocabulary.md](record-vocabulary.md) |
| Sales object | deal | `thương vụ`; giao dịch or cơ hội for a deal (email giao dịch, tên giao dịch and the company lifecycle stage keep theirs) |
| Lead | lead | khách hàng tiềm năng or khách tiềm năng for a lead (the Prospect lifecycle stage keeps it) |
| Ordered stages | pipeline | `phễu`; quy trình bán hàng for the pipeline |
| A pipeline step | giai đoạn | chặng for a step (chặng đường is ordinary Vietnamese) |
| Forecast object | dự báo | `dự phóng`, `forecast` |
| Forecast categories | Commit, Best case | `trường hợp tốt nhất`, `khả quan nhất`; Cam kết for the category |
| Manager's forecast number | nhận định (Nhận định của quản lý) | đánh giá or dự báo của quản lý for this number |
| Something a party said they would do | cam kết | `lời hứa` |
| The reader's queue | Danh sách công việc | `worklist`, `hàng đợi công việc`; hộp thư đến for the Worklist |
| Daily digest | Bản tin sáng | `bản tóm tắt buổi sáng`, `briefing` |
| Approvals surface; approve | Phê duyệt; duyệt, đã duyệt | `chấp thuận` |
| A classifier's result | kết quả, lần kiểm tra | `phán quyết`, `verdict` |
| Capture intake step | kiểm tra đầu vào | `admission check` |
| Ownership of a deal | phụ trách (label Phụ trách) | người sở hữu for ownership |
| Tabs of a record | the tab labels themselves | `phần của bản ghi` |
| The running Margince system | bản cài đặt | |
| Administrator; member; users; staff | quản trị viên; thành viên; người dùng; nhân viên, nhân viên kinh doanh | |
| Agent credential | passport | `hộ chiếu`; token or khóa for a passport (khóa API and khóa ký name other credentials) |
| Mail or calendar link | trình kết nối | `bộ kết nối`, `connector`; kết nối alone or tích hợp for a connector |
| Buyer-facing deal page | Deal Room | `phòng deal`; cổng thông tin for the Deal Room |
| The mail thread of a record | chuỗi thư | `spine` |
| Import of mailbox history | nhập lịch sử hộp thư | `backread`; đọc ngược for the import |
| Full read of a web page | đọc toàn trang | `đọc sâu`, `deep read` |
| The AI agent | agent | `tác tử`; trợ lý (that is assistant) |
| Artificial intelligence | AI | `TTNT` |
| Sign in, sign out | đăng nhập, đăng xuất | `login`, `log in`, `đăng nhập vào trong` |
| Retry | Thử lại | `thử lần nữa` |
| Upload, download | tải lên, tải xuống | `upload`, `download` |
| Email | email | `thư điện tử`, `e-mail` |
| Mailbox | hộp thư | `mailbox` |
| Meeting | cuộc họp | `buổi họp`, `meeting` |
| Task | việc cần làm; a background job is tác vụ | `task`; nhiệm vụ or công việc for a task |
| Tag | thẻ | `tag`; nhãn for a tag |
| Report | báo cáo | `report` |
| AI quota | hạn mức | `định mức` |
| Licensed seat | suất (suất đầy đủ, suất chỉ đọc) | `chỗ ngồi`, `ghế` |
| Legal hold | lưu giữ pháp lý | `legal hold`; nghĩa vụ lưu giữ for the hold (a statutory retention obligation keeps it) |
| Buying committee, buying center | nhóm quyết định mua | `buying center`, `nhóm mua hàng` |
| Audit log | nhật ký kiểm tra | `nhật ký kiểm toán`, `audit log`, `audit trail` |
| Follow-up | việc tiếp theo | `follow-up`; theo dõi for a follow-up (it is watch) |
| Priced offer, quote | báo giá | `chào giá` |
| Close date; close a deal | ngày chốt; chốt | `ngày đóng` |
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
| Remove, delete; archive, retention | gỡ, xóa; lưu trữ, lưu giữ | |
| Writing voice, its profile, its corpus; signing secret; champion | giọng văn, hồ sơ giọng văn, kho văn mẫu; khóa ký; người ủng hộ | |

**người** appears only where a human being is meant, and then the key is on the
Vietnamese human-sense list in `record-noun.test.ts`. Prefer thành viên, quản
trị viên or ai where they read naturally.

## Contested choices

- **tổ chức and công ty**: English "company" names both the tenant and the
  record; two words keep the reader's installation apart from a customer.
- **Kiểu cũ**: the product owner's call. Both styles are correct; mixing is not.
- **Danh sách công việc, Bản tin sáng, nhận định**: the loanwords are deal,
  lead and pipeline only; đánh giá is a rating, dự báo the forecast.
- **Commit, Best case, agent, passport, endpoint stay English**: cam kết is the
  commitment, "tốt nhất" a regulated superlative, trợ lý an assistant and hộ
  chiếu a travel document.
- **trình kết nối, việc cần làm, thẻ, nhật ký kiểm tra**: Google's and
  Microsoft's Vietnamese; a UI card is "khung", so thẻ stays unambiguous, and
  kiểm toán is a financial audit.
- **suất** is the countable licensed slot; chỗ ngồi is a chair.
- **lưu giữ pháp lý, nhóm quyết định mua, việc tiếp theo**: no settled loanword,
  and theo dõi is watch, which the product also does.
- **máy chủ, tệp, trang web**: the reader is a sales team, not engineers;
  thu thập is the verb of Nghị định 13/2023/NĐ-CP on data protection.

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
| Retired vocabulary | A word in code format in the Vocabulary table's Never column |

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
addressing, mixed address on one surface, slot grammar, end punctuation and
edge spaces against the English, sentence case, hand-formatted numbers,
length, or the plain words of the Never column. Those are the author's and the
reviewer's judgement.
