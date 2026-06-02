package resume

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/ledongthuc/pdf"
)

// Parse extracts human-readable text from a resume file. It detects the file
// format via MIME type and applies the appropriate text extraction.
func Parse(fileData []byte) (string, error) {
	if len(fileData) == 0 {
		return "", fmt.Errorf("empty file")
	}

	mime := mimetype.Detect(fileData)
	switch mime.String() {
	case "application/pdf":
		return extractPDFText(fileData)
	case "text/plain", "text/markdown", "text/html":
		return string(fileData), nil
	default:
		// For unknown types, check if it looks like text
		if isPlainText(fileData) {
			return string(fileData), nil
		}
		return "", fmt.Errorf(
			"unsupported file type %q — please upload a PDF or paste resume text directly",
			mime.String(),
		)
	}
}

func extractPDFText(fileData []byte) (string, error) {
	reader := bytes.NewReader(fileData)
	size := reader.Size()

	pdfReader, err := pdf.NewReader(reader, size)
	if err != nil {
		return "", fmt.Errorf("cannot open PDF: %w", err)
	}

	var buf strings.Builder
	totalPage := pdfReader.NumPage()

	for pageNum := 1; pageNum <= totalPage; pageNum++ {
		page := pdfReader.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteByte('\n')
	}

	result := strings.TrimSpace(buf.String())
	if result == "" {
		return "", fmt.Errorf(
			"no extractable text found in PDF — the file may contain only scanned images; please paste resume text directly",
		)
	}
	return result, nil
}

// isPlainText checks whether binary data is likely plain text by looking for
// null bytes and control characters.
func isPlainText(data []byte) bool {
	if len(data) > 4096 {
		data = data[:4096]
	}
	for i, b := range data {
		if b == 0 {
			return false
		}
		if i > 512 {
			break
		}
	}
	return true
}
