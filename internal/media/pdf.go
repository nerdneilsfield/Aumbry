package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const (
	// MaxPDFBytes enforces the PRD §5.3 hard size cap (50 MiB) on uploaded PDF attachments.
	MaxPDFBytes = 50 << 20

	pdfHeaderProbeBytes = 1024
	pdfParseTimeout     = 20 * time.Second
)

// ErrInvalidPDF indicates that an uploaded attachment is not a readable, non-user-encrypted PDF.
var ErrInvalidPDF = errors.New("invalid pdf")

// PDFMetadata holds validated container metadata extracted from a PDF attachment.
type PDFMetadata struct {
	PageCount int
}

// InspectPDF verifies that rs contains a structurally valid PDF document within MaxPDFBytes and
// extracts its page count using a pure-Go stateless pdfcpu configuration. PDFs protected only by
// an owner password (empty user password) succeed; user-password encrypted, malformed, or 0-page
// PDFs return ErrInvalidPDF.
func InspectPDF(ctx context.Context, rs io.ReadSeeker, detectedMIME string) (meta PDFMetadata, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return PDFMetadata{}, fmt.Errorf("%w: %w", ErrInvalidPDF, ctxErr)
	}
	if detectedMIME != "application/pdf" {
		return PDFMetadata{}, fmt.Errorf("%w: unexpected mime type %q", ErrInvalidPDF, detectedMIME)
	}
	if rs == nil {
		return PDFMetadata{}, fmt.Errorf("%w: empty pdf stream", ErrInvalidPDF)
	}
	size, seekErr := rs.Seek(0, io.SeekEnd)
	if seekErr != nil {
		return PDFMetadata{}, fmt.Errorf("%w: seek end: %w", ErrInvalidPDF, seekErr)
	}
	if size <= 0 || size > MaxPDFBytes {
		return PDFMetadata{}, fmt.Errorf("%w: pdf size %d out of bounds", ErrInvalidPDF, size)
	}
	if _, seekErr = rs.Seek(0, io.SeekStart); seekErr != nil {
		return PDFMetadata{}, fmt.Errorf("%w: seek start: %w", ErrInvalidPDF, seekErr)
	}

	probeLen := min(int(size), pdfHeaderProbeBytes)
	head := make([]byte, probeLen)
	if _, readErr := io.ReadFull(rs, head); readErr != nil {
		return PDFMetadata{}, fmt.Errorf("%w: read pdf header: %w", ErrInvalidPDF, readErr)
	}
	if !bytes.Contains(head, []byte("%PDF-")) {
		return PDFMetadata{}, fmt.Errorf("%w: missing %%PDF- header", ErrInvalidPDF)
	}
	if _, seekErr = rs.Seek(0, io.SeekStart); seekErr != nil {
		return PDFMetadata{}, fmt.Errorf("%w: rewind pdf: %w", ErrInvalidPDF, seekErr)
	}

	defer func() {
		if r := recover(); r != nil {
			meta = PDFMetadata{}
			err = fmt.Errorf("%w: parser panic: %v", ErrInvalidPDF, r)
		}
	}()

	conf := model.NewStatelessConfiguration()
	conf.Offline = true
	conf.ValidationMode = model.ValidationRelaxed
	conf.ValidateLinks = false
	conf.CheckFileNameExt = false
	conf.Limits.MaxObjectBytes = 32 << 20
	conf.Limits.MaxStreamBytes = 64 << 20
	conf.Limits.MaxDecodeBytes = 64 << 20
	conf.Limits.MaxImageBytes = 64 << 20
	conf.Limits.MaxObjectCount = 500_000
	conf.Limits.MaxObjectStreamCount = 100_000
	conf.Limits.MaxXRefEntries = 500_000
	conf.Limits.MaxRecursionDepth = 100

	parseCtx, cancel := context.WithTimeout(ctx, pdfParseTimeout)
	defer cancel()

	pages, countErr := pdfcpuapi.PageCount(parseCtx, rs, conf)
	if countErr != nil {
		return PDFMetadata{}, fmt.Errorf("%w: %w", ErrInvalidPDF, countErr)
	}
	if pages < 1 {
		return PDFMetadata{}, fmt.Errorf("%w: pdf has %d pages", ErrInvalidPDF, pages)
	}
	return PDFMetadata{PageCount: pages}, nil
}
