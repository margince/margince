// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"encoding/csv"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// FileFault is a whole file an import refuses. Params let a screen word it, and
// "reason" tells apart the causes one Kind covers.
type FileFault struct {
	Kind    error
	Message string
	Params  Params
	cause   error
}

// Error is the log line, with the CSV or ZIP reader's own failure when there is one.
func (f *FileFault) Error() string {
	if f.cause != nil {
		return fmt.Sprintf("%v: %s (%v)", f.Kind, f.Message, f.cause)
	}
	return fmt.Sprintf("%v: %s", f.Kind, f.Message)
}

// Unwrap answers the sentinel, so errors.Is sorts a fault by its kind.
func (f *FileFault) Unwrap() error { return f.Kind }

// FileReason tells apart the causes one FileFault Kind covers.
type FileReason string

// The reasons a file is refused.
const (
	FileFieldCount FileReason = "field_count"
	FileQuote      FileReason = "quote"

	FileNoHeader     FileReason = "no_header"
	FileBlankName    FileReason = "blank_name"
	FileRepeatedName FileReason = "repeated_name"

	BundleNotAZip           FileReason = "not_a_zip"
	BundleEmpty             FileReason = "empty"
	BundleFilesTooLarge     FileReason = "files_too_large"
	BundleZipTooLarge       FileReason = "zip_too_large"
	BundleTooManyFiles      FileReason = "too_many_files"
	BundlePasswordProtected FileReason = "password_protected"
	BundleTwoFilesOneType   FileReason = "two_files_one_type"
	BundleFileDamaged       FileReason = "file_damaged"
	BundleFileMissing       FileReason = "file_missing"
	BundleTypeNotImportable FileReason = "type_not_importable"
)

// fileReasonKinds maps each reason to the sentinel it refines. A fault takes
// its Kind from here, so a reason missing from it refuses as no kind at all.
var fileReasonKinds = map[FileReason]error{
	FileFieldCount: ErrSourceUnreadable, FileQuote: ErrSourceUnreadable,
	FileNoHeader: ErrHeaderInvalid, FileBlankName: ErrHeaderInvalid, FileRepeatedName: ErrHeaderInvalid,
	BundleNotAZip: ErrBundleRefused, BundleEmpty: ErrBundleRefused, BundleFilesTooLarge: ErrBundleRefused,
	BundleZipTooLarge: ErrBundleRefused, BundleTooManyFiles: ErrBundleRefused, BundlePasswordProtected: ErrBundleRefused,
	BundleTwoFilesOneType: ErrBundleRefused, BundleFileDamaged: ErrBundleRefused, BundleFileMissing: ErrBundleRefused,
	BundleTypeNotImportable: ErrBundleRefused,
}

// FileFaultReasons answers every reason a file refusal may carry, sorted.
func FileFaultReasons() []FileReason {
	return slices.Sorted(maps.Keys(fileReasonKinds))
}

// FileReasonKind is the sentinel a reason refines, or nil for one no fault carries.
func FileReasonKind(reason FileReason) error { return fileReasonKinds[reason] }

// refuseFile is a fault for reason, of the kind the reason refines.
func refuseFile(reason FileReason, message string, values map[RefusalParam]string) *FileFault {
	fault := refuseFileAs(fileReasonKinds[reason], message, values)
	fault.Params[string(ParamReason)] = string(reason)
	return fault
}

// refuseFileAs is a fault of kind whose kind alone says what is wrong.
func refuseFileAs(kind error, message string, values map[RefusalParam]string) *FileFault {
	return &FileFault{Kind: kind, Message: message, Params: RefusalValues(values)}
}

// unreadable words a CSV reader's failure. A parse error names its line; any
// other failure is a read of the stored bytes, which names none.
func unreadable(err error) error {
	if inner, ok := errors.AsType[*FileFault](err); ok {
		return inner
	}
	parse, ok := errors.AsType[*csv.ParseError](err)
	if !ok {
		fault := refuseFileAs(ErrSourceUnreadable, "The file could not be read. Upload it again.", nil)
		fault.cause = err
		return fault
	}
	line := map[RefusalParam]string{ParamLine: strconv.Itoa(parse.Line)}
	var fault *FileFault
	switch {
	case errors.Is(parse.Err, csv.ErrFieldCount):
		fault = refuseFile(FileFieldCount,
			fmt.Sprintf("Line %d has more or fewer fields than the header row. Fix that line and upload the file again.", parse.Line), line)
	case errors.Is(parse.Err, csv.ErrQuote), errors.Is(parse.Err, csv.ErrBareQuote):
		fault = refuseFile(FileQuote,
			fmt.Sprintf("Line %d has a quotation mark out of place. Fix that line and upload the file again.", parse.Line), line)
	default:
		fault = refuseFileAs(ErrSourceUnreadable,
			fmt.Sprintf("Line %d could not be read. Fix that line and upload the file again.", parse.Line), line)
	}
	fault.cause = err
	return fault
}

func headerMissing() error {
	return refuseFile(FileNoHeader,
		"The file has no header row. Put the column names on the first line and upload it again.", nil)
}

// InEntry names the ZIP entry an error is about, on a FileFault as its "file".
func InEntry(entry string, err error) error {
	fault, ok := errors.AsType[*FileFault](err)
	if !ok {
		return fmt.Errorf("%s: %w", entry, err)
	}
	named := *fault
	named.Params = maps.Clone(fault.Params)
	if named.Params == nil {
		named.Params = Params{}
	}
	named.Params[string(ParamFile)] = entry
	first, size := utf8.DecodeRuneInString(fault.Message)
	named.Message = fmt.Sprintf("In “%s”, %c%s", entry, unicode.ToLower(first), fault.Message[size:])
	return &named
}
