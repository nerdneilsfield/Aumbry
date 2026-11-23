package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	pdfcpuapi "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// buildTestPDF constructs a minimal valid PDF 1.4 document with n blank Letter pages.
func buildTestPDF(t *testing.T, pages int) []byte {
	t.Helper()
	if pages < 1 {
		pages = 1
	}
	var kids []string
	var objs []string
	// 1: Catalog, 2: Pages, 3..N+2: Page objects
	for i := range pages {
		objNum := 3 + i
		kids = append(kids, fmt.Sprintf("%d 0 R", objNum))
		objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", objNum))
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, pages+2)
	addObj := func(body string) {
		offsets = append(offsets, buf.Len())
		buf.WriteString(body)
	}
	addObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	addObj(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), pages))
	for _, o := range objs {
		addObj(o)
	}
	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xrefOffset)
	return buf.Bytes()
}

func encryptTestPDF(t *testing.T, raw []byte, userPW, ownerPW string) []byte {
	t.Helper()
	conf := model.NewAESConfiguration(userPW, ownerPW, 256)
	conf.Offline = true
	conf.ValidationMode = model.ValidationRelaxed
	var out bytes.Buffer
	if err := pdfcpuapi.Encrypt(context.Background(), bytes.NewReader(raw), &out, conf); err != nil {
		t.Fatalf("Encrypt test PDF: %v", err)
	}
	return out.Bytes()
}

func TestInspectPDF(t *testing.T) {
	raw3 := buildTestPDF(t, 3)

	t.Run("valid multi-page PDF returns exact page count", func(t *testing.T) {
		meta, err := InspectPDF(t.Context(), bytes.NewReader(raw3), "application/pdf")
		if err != nil {
			t.Fatalf("InspectPDF(3-page) = %v", err)
		}
		if meta.PageCount != 3 {
			t.Fatalf("PageCount = %d, want 3", meta.PageCount)
		}
	})

	t.Run("owner-password only PDF (empty user password) succeeds", func(t *testing.T) {
		ownerOnly := encryptTestPDF(t, raw3, "", "owner-secret")
		meta, err := InspectPDF(t.Context(), bytes.NewReader(ownerOnly), "application/pdf")
		if err != nil {
			t.Fatalf("InspectPDF(owner-only encrypted) = %v", err)
		}
		if meta.PageCount != 3 {
			t.Fatalf("PageCount = %d, want 3", meta.PageCount)
		}
	})

	t.Run("user-password encrypted PDF is rejected", func(t *testing.T) {
		userLocked := encryptTestPDF(t, raw3, "user-secret", "owner-secret")
		if _, err := InspectPDF(t.Context(), bytes.NewReader(userLocked), "application/pdf"); !errors.Is(err, ErrInvalidPDF) {
			t.Fatalf("InspectPDF(user-locked) = %v, want ErrInvalidPDF", err)
		}
	})

	t.Run("corrupt, canceled ctx, or mismatched MIME rejected", func(t *testing.T) {
		if _, err := InspectPDF(t.Context(), bytes.NewReader([]byte("%PDF-1.4\nbad")), "application/pdf"); !errors.Is(err, ErrInvalidPDF) {
			t.Fatalf("InspectPDF(corrupt) = %v, want ErrInvalidPDF", err)
		}
		if _, err := InspectPDF(t.Context(), bytes.NewReader(raw3), "text/plain"); !errors.Is(err, ErrInvalidPDF) {
			t.Fatalf("InspectPDF(wrong mime) = %v, want ErrInvalidPDF", err)
		}
		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := InspectPDF(canceled, bytes.NewReader(raw3), "application/pdf"); !errors.Is(err, ErrInvalidPDF) {
			t.Fatalf("InspectPDF(canceled) = %v, want ErrInvalidPDF", err)
		}
	})
}
