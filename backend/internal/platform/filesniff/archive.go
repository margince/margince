// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package filesniff

import (
	"archive/zip"
	"io"
	"strings"
)

// maxMimetypeBytes bounds the ODF "mimetype" entry read; the real ones are
// under 100 bytes.
const maxMimetypeBytes = 256

// ooxmlMainParts maps each Office Open XML type to the folder of its main part.
var ooxmlMainParts = map[string]string{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   "word/",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xl/",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "ppt/",
}

// odfTypes are the OpenDocument types, each named by its own "mimetype" entry.
var odfTypes = map[string]bool{
	"application/vnd.oasis.opendocument.text":         true,
	"application/vnd.oasis.opendocument.spreadsheet":  true,
	"application/vnd.oasis.opendocument.presentation": true,
}

// readerAt lets archive/zip read through the ReadSeeker an upload is. The
// uploads are read by one goroutine, so seeking per read is safe.
type readerAt struct{ io.ReadSeeker }

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if _, err := r.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	n, err := io.ReadFull(r, p)
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}

// openZip reads the central directory only, so the cost is bounded by the
// archive's entry table and not by its contents.
func openZip(f file) (*zip.Reader, bool) {
	size, err := f.content.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, false
	}
	zr, err := zip.NewReader(readerAt{f.content}, size)
	return zr, err == nil
}

// isProgramArchive recognises a JAR or an APK, which are zips that run.
func isProgramArchive(zr *zip.Reader) bool {
	for _, entry := range zr.File {
		name := entry.Name
		if name == "AndroidManifest.xml" || name == "classes.dex" || strings.HasSuffix(name, ".class") {
			return true
		}
	}
	return false
}

func hasEntry(zr *zip.Reader, name string) *zip.File {
	for _, entry := range zr.File {
		if entry.Name == name {
			return entry
		}
	}
	return nil
}

func isPlainZip(f file) bool {
	zr, ok := openZip(f)
	return ok && !isProgramArchive(zr)
}

// ooxmlRule wants [Content_Types].xml and a part in the type's own folder.
// A docx is then no renamed xlsx, and a bare zip is neither.
func ooxmlRule(folder string) func(file) bool {
	return func(f file) bool {
		zr, ok := openZip(f)
		if !ok || isProgramArchive(zr) || hasEntry(zr, "[Content_Types].xml") == nil {
			return false
		}
		for _, entry := range zr.File {
			if strings.HasPrefix(entry.Name, folder) {
				return true
			}
		}
		return false
	}
}

// odfRule wants the "mimetype" entry to name the type the file claims.
func odfRule(mediaType string) func(file) bool {
	return func(f file) bool {
		zr, ok := openZip(f)
		if !ok || isProgramArchive(zr) {
			return false
		}
		entry := hasEntry(zr, "mimetype")
		if entry == nil {
			return false
		}
		rc, err := entry.Open()
		if err != nil {
			return false
		}
		raw, readErr := io.ReadAll(io.LimitReader(rc, maxMimetypeBytes))
		closeErr := rc.Close()
		return readErr == nil && closeErr == nil && strings.TrimSpace(string(raw)) == mediaType
	}
}
