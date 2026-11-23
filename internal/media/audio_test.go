package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/abema/go-mp4"
	"github.com/gabriel-vasile/mimetype"
)

func buildValidWAV(t *testing.T, secs int) []byte {
	t.Helper()
	const sampleRate = 8000
	const channels = 1
	const bitsPerSample = 16
	byteRate := sampleRate * channels * (bitsPerSample / 8)
	dataLen := secs * byteRate

	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels*(bitsPerSample/8)))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataLen))
	buf.Write(make([]byte, dataLen))
	return buf.Bytes()
}

func buildValidMP3(t *testing.T, frames int, withID3 bool) []byte {
	t.Helper()
	// MPEG-1 Layer III, 128 kbps, 44100 Hz, mono, no CRC, no padding -> 417 bytes/frame, ~26.122ms/frame.
	const frameSize = 417
	frame := make([]byte, frameSize)
	frame[0] = 0xFF
	frame[1] = 0xFB
	frame[2] = 0x90
	frame[3] = 0xC0

	buf := &bytes.Buffer{}
	if withID3 {
		// ID3v2.3 header with 10-byte payload (20 bytes total).
		buf.Write([]byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 10})
		buf.Write(make([]byte, 10))
	}
	for range frames {
		buf.Write(frame)
	}
	return buf.Bytes()
}

type syntheticMP4Opts struct {
	handlerType  [4]byte
	timescale    uint32
	duration     uint32
	mvhdDuration uint32
	omitStbl     bool
	sampleCount  uint32
	emptyMdat    bool
	hdlrPadLen   int
}

func buildSyntheticMP4(t *testing.T, handlerType [4]byte, timescale uint32, duration uint32, mvhdDuration uint32) []byte {
	return buildSyntheticMP4WithOpts(t, syntheticMP4Opts{
		handlerType:  handlerType,
		timescale:    timescale,
		duration:     duration,
		mvhdDuration: mvhdDuration,
		sampleCount:  1,
	})
}

func buildSyntheticMP4WithOpts(t *testing.T, opts syntheticMP4Opts) []byte {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.m4a")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	w := mp4.NewWriter(f)
	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeFtyp()}); err != nil {
		t.Fatal(err)
	}
	if _, err = mp4.Marshal(w, &mp4.Ftyp{
		MajorBrand:   [4]byte{'M', '4', 'A', ' '},
		MinorVersion: 0,
		CompatibleBrands: []mp4.CompatibleBrandElem{
			{CompatibleBrand: [4]byte{'M', '4', 'A', ' '}},
			{CompatibleBrand: [4]byte{'i', 's', 'o', 'm'}},
		},
	}, mp4.Context{}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMoov()}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMvhd()}); err != nil {
		t.Fatal(err)
	}
	if _, err = mp4.Marshal(w, &mp4.Mvhd{
		Timescale:   1000,
		DurationV0:  opts.mvhdDuration,
		Rate:        0x00010000,
		Volume:      0x0100,
		NextTrackID: 2,
	}, mp4.Context{}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeTrak()}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdia()}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdhd()}); err != nil {
		t.Fatal(err)
	}
	if _, err = mp4.Marshal(w, &mp4.Mdhd{
		Timescale:  opts.timescale,
		DurationV0: opts.duration,
	}, mp4.Context{}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}
	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeHdlr()}); err != nil {
		t.Fatal(err)
	}
	if opts.hdlrPadLen > 0 {
		pad := make([]byte, 24+opts.hdlrPadLen)
		copy(pad[8:12], opts.handlerType[:])
		if _, err = w.Write(pad); err != nil {
			t.Fatal(err)
		}
	} else if _, err = mp4.Marshal(w, &mp4.Hdlr{
		HandlerType: opts.handlerType,
		Name:        "TrackHandler",
	}, mp4.Context{}); err != nil {
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}

	if !opts.omitStbl {
		if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMinf()}); err != nil {
			t.Fatal(err)
		}
		if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeStbl()}); err != nil {
			t.Fatal(err)
		}
		if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeStsd()}); err != nil {
			t.Fatal(err)
		}
		// FullBox (4) + entry_count=1 (4) + sample entry size=16 (4) + "mp4a" (4) + 8-byte stub
		var stsd [24]byte
		binary.BigEndian.PutUint32(stsd[4:8], 1)
		binary.BigEndian.PutUint32(stsd[8:12], 16)
		copy(stsd[12:16], []byte("mp4a"))
		if _, err = w.Write(stsd[:]); err != nil {
			t.Fatal(err)
		}
		if _, err = w.EndBox(); err != nil {
			t.Fatal(err)
		}

		if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeStsz()}); err != nil {
			t.Fatal(err)
		}
		// FullBox (4) + sample_size=1024 (4) + sample_count (4)
		var stsz [12]byte
		binary.BigEndian.PutUint32(stsz[4:8], 1024)
		binary.BigEndian.PutUint32(stsz[8:12], opts.sampleCount)
		if _, err = w.Write(stsz[:]); err != nil {
			t.Fatal(err)
		}
		if _, err = w.EndBox(); err != nil {
			t.Fatal(err)
		}

		if _, err = w.EndBox(); err != nil { // stbl
			t.Fatal(err)
		}
		if _, err = w.EndBox(); err != nil { // minf
			t.Fatal(err)
		}
	}

	if _, err = w.EndBox(); err != nil { // mdia
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil { // trak
		t.Fatal(err)
	}
	if _, err = w.EndBox(); err != nil { // moov
		t.Fatal(err)
	}

	if _, err = w.StartBox(&mp4.BoxInfo{Type: mp4.BoxTypeMdat()}); err != nil {
		t.Fatal(err)
	}
	if !opts.emptyMdat {
		if _, err = w.Write([]byte("audio-data")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = w.EndBox(); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestInspectAudioValidFormatsAndMIME(t *testing.T) {
	t.Run("wav 2s", func(t *testing.T) {
		raw := buildValidWAV(t, 2)
		mime := mimetype.Detect(raw).String()
		meta, err := InspectAudio(bytes.NewReader(raw), ".wav", mime)
		if err != nil {
			t.Fatalf("InspectAudio(.wav, %q) = %v", mime, err)
		}
		if meta.MIMEType != "audio/wav" || meta.DurationSeconds == nil || *meta.DurationSeconds != 2 {
			t.Fatalf("meta = %+v, want audio/wav and 2s", meta)
		}
	})

	t.Run("mp3 raw and id3v2", func(t *testing.T) {
		for _, withID3 := range []bool{false, true} {
			raw := buildValidMP3(t, 40, withID3) // ~1.04s -> 1s
			mime := mimetype.Detect(raw).String()
			meta, err := InspectAudio(bytes.NewReader(raw), ".mp3", mime)
			if err != nil {
				t.Fatalf("InspectAudio(.mp3, withID3=%v, %q) = %v", withID3, mime, err)
			}
			if meta.MIMEType != "audio/mpeg" || meta.DurationSeconds == nil || *meta.DurationSeconds != 1 {
				t.Fatalf("meta = %+v, want audio/mpeg and 1s", meta)
			}
		}
	})

	t.Run("mp3 hitting frame cap omits duration instead of underreporting", func(t *testing.T) {
		prev := maxMP3Frames
		maxMP3Frames = 5
		t.Cleanup(func() { maxMP3Frames = prev })

		raw := buildValidMP3(t, 40, false)
		meta, err := InspectAudio(bytes.NewReader(raw), ".mp3", "audio/mpeg")
		if err != nil {
			t.Fatalf("InspectAudio capped mp3 = %v", err)
		}
		if meta.MIMEType != "audio/mpeg" || meta.DurationSeconds != nil {
			t.Fatalf("capped mp3 meta = %+v, want nil DurationSeconds", meta)
		}
	})

	t.Run("m4a sound track and unknown zero duration", func(t *testing.T) {
		raw := buildSyntheticMP4(t, [4]byte{'s', 'o', 'u', 'n'}, 44100, 132300, 3000)
		mime := mimetype.Detect(raw).String()
		meta, err := InspectAudio(bytes.NewReader(raw), ".m4a", mime)
		if err != nil {
			t.Fatalf("InspectAudio(.m4a, %q) = %v", mime, err)
		}
		if meta.MIMEType != "audio/mp4" || meta.DurationSeconds == nil || *meta.DurationSeconds != 3 {
			t.Fatalf("meta = %+v, want audio/mp4 and 3s", meta)
		}

		noDur := buildSyntheticMP4(t, [4]byte{'s', 'o', 'u', 'n'}, 44100, 0, 0)
		mimeNoDur := mimetype.Detect(noDur).String()
		metaNoDur, err := InspectAudio(bytes.NewReader(noDur), ".m4a", mimeNoDur)
		if err != nil {
			t.Fatalf("InspectAudio(.m4a noDur) = %v", err)
		}
		if metaNoDur.MIMEType != "audio/mp4" || metaNoDur.DurationSeconds != nil {
			t.Fatalf("metaNoDur = %+v, want nil duration", metaNoDur)
		}
	})
}

func TestInspectAudioRejectsInvalidAndVideoOnlyFiles(t *testing.T) {
	t.Run("video-only mp4 renamed to m4a rejected", func(t *testing.T) {
		raw := buildSyntheticMP4(t, [4]byte{'v', 'i', 'd', 'e'}, 90000, 270000, 3000)
		mime := mimetype.Detect(raw).String()
		if _, err := InspectAudio(bytes.NewReader(raw), ".m4a", mime); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("video-only .m4a error = %v, want ErrInvalidAudio", err)
		}
	})

	t.Run("m4a missing sample table, 0 samples, empty mdat, or oversized hdlr rejected", func(t *testing.T) {
		noStbl := buildSyntheticMP4WithOpts(t, syntheticMP4Opts{
			handlerType: [4]byte{'s', 'o', 'u', 'n'}, timescale: 44100, duration: 44100, omitStbl: true,
		})
		if _, err := InspectAudio(bytes.NewReader(noStbl), ".m4a", "audio/mp4"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("sample-less stbl m4a = %v, want ErrInvalidAudio", err)
		}

		zeroSamples := buildSyntheticMP4WithOpts(t, syntheticMP4Opts{
			handlerType: [4]byte{'s', 'o', 'u', 'n'}, timescale: 44100, duration: 44100, sampleCount: 0,
		})
		if _, err := InspectAudio(bytes.NewReader(zeroSamples), ".m4a", "audio/mp4"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("zero-sample m4a = %v, want ErrInvalidAudio", err)
		}

		emptyMdat := buildSyntheticMP4WithOpts(t, syntheticMP4Opts{
			handlerType: [4]byte{'s', 'o', 'u', 'n'}, timescale: 44100, duration: 44100, sampleCount: 1, emptyMdat: true,
		})
		if _, err := InspectAudio(bytes.NewReader(emptyMdat), ".m4a", "audio/mp4"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("empty-mdat m4a = %v, want ErrInvalidAudio", err)
		}

		hugeHdlr := buildSyntheticMP4WithOpts(t, syntheticMP4Opts{
			handlerType: [4]byte{'s', 'o', 'u', 'n'}, timescale: 44100, duration: 44100, sampleCount: 1, hdlrPadLen: maxMP4MetaBoxBytes + 64,
		})
		if _, err := InspectAudio(bytes.NewReader(hugeHdlr), ".m4a", "audio/mp4"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("oversized hdlr m4a = %v, want ErrInvalidAudio", err)
		}
	})

	t.Run("wav unaligned PCM data or bad RIFF bounds rejected", func(t *testing.T) {
		valid := buildValidWAV(t, 1)
		// Corrupt RIFF length to exceed actual file size.
		badRIFF := append([]byte(nil), valid...)
		binary.LittleEndian.PutUint32(badRIFF[4:8], uint32(len(badRIFF)+100))
		if _, err := InspectAudio(bytes.NewReader(badRIFF), ".wav", "audio/wav"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("bad RIFF size wav = %v, want ErrInvalidAudio", err)
		}

		// PCM with blockAlign=4 (stereo 16-bit) but data length=1 byte -> rejected.
		unaligned := &bytes.Buffer{}
		unaligned.WriteString("RIFF")
		_ = binary.Write(unaligned, binary.LittleEndian, uint32(36+2))
		unaligned.WriteString("WAVEfmt ")
		_ = binary.Write(unaligned, binary.LittleEndian, uint32(16))
		_ = binary.Write(unaligned, binary.LittleEndian, uint16(1))
		_ = binary.Write(unaligned, binary.LittleEndian, uint16(2))
		_ = binary.Write(unaligned, binary.LittleEndian, uint32(8000))
		_ = binary.Write(unaligned, binary.LittleEndian, uint32(32000))
		_ = binary.Write(unaligned, binary.LittleEndian, uint16(4))
		_ = binary.Write(unaligned, binary.LittleEndian, uint16(16))
		unaligned.WriteString("data")
		_ = binary.Write(unaligned, binary.LittleEndian, uint32(1))
		unaligned.Write([]byte{0, 0})
		if _, err := InspectAudio(bytes.NewReader(unaligned.Bytes()), ".wav", "audio/wav"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("unaligned 1-byte PCM wav = %v, want ErrInvalidAudio", err)
		}
	})

	t.Run("corrupt payloads and mime mismatches rejected", func(t *testing.T) {
		if _, err := InspectAudio(bytes.NewReader([]byte("RIFF\x20\x00\x00\x00WAVE")), ".wav", "audio/wav"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("truncated wav = %v, want ErrInvalidAudio", err)
		}
		if _, err := InspectAudio(bytes.NewReader([]byte("not-mp3-data")), ".mp3", "audio/mpeg"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("corrupt mp3 = %v, want ErrInvalidAudio", err)
		}
		if _, err := InspectAudio(bytes.NewReader([]byte("\x00\x00\x00\x08ftyp")), ".m4a", "audio/mp4"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("corrupt m4a = %v, want ErrInvalidAudio", err)
		}
		wav := buildValidWAV(t, 1)
		if _, err := InspectAudio(bytes.NewReader(wav), ".mp3", "audio/wav"); !errors.Is(err, ErrInvalidAudio) {
			t.Fatalf("mime mismatch = %v, want ErrInvalidAudio", err)
		}
	})

	t.Run("unsupported extensions rejected", func(t *testing.T) {
		for _, ext := range []string{".ogg", ".flac", ".aac"} {
			if _, err := InspectAudio(bytes.NewReader([]byte("OggS")), ext, "audio/ogg"); !errors.Is(err, ErrUnsupportedAudio) {
				t.Fatalf("ext %s = %v, want ErrUnsupportedAudio", ext, err)
			}
		}
	})
}
