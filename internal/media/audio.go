// Package media validates uploaded binary media assets and extracts metadata using pure-Go parsers.
package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/abema/go-mp4"
	"github.com/tcolgate/mp3"
)

const (
	maxWAVChunks        = 256
	maxMP3InitialSkip   = 128 * 1024
	maxMP4Boxes         = 10_000
	maxMP4BoxDepth      = 16
	maxMP4MetaBoxBytes  = 1 << 20 // 1 MiB cap on any metadata box inspected into memory
	maxID3v2HeaderBytes = 64 * 1024 * 1024
)

// Variable so unit tests can verify truncation of duration at the frame limit without allocating 500k frames.
var maxMP3Frames = 500_000

var (
	// ErrInvalidAudio reports a malformed or structurally inconsistent audio asset (including video-only MP4 disguised as M4A).
	ErrInvalidAudio = errors.New("media: invalid audio file")
	// ErrUnsupportedAudio reports an unrecognized or unsupported audio container extension or MIME type.
	ErrUnsupportedAudio = errors.New("media: unsupported audio format")
)

// AudioMetadata holds canonical container metadata extracted from a validated audio asset.
type AudioMetadata struct {
	MIMEType        string
	DurationSeconds *int
}

// InspectAudio validates an uploaded audio stream against ext and detectedMIME and returns
// its canonical MIME type and optional rounded duration in seconds.
func InspectAudio(r io.ReadSeeker, ext, detectedMIME string) (AudioMetadata, error) {
	if r == nil {
		return AudioMetadata{}, fmt.Errorf("%w: nil reader", ErrInvalidAudio)
	}
	normExt := strings.ToLower(strings.TrimSpace(ext))
	normMIME := normalizeMIME(detectedMIME)

	switch normExt {
	case ".mp3":
		if normMIME != "audio/mpeg" {
			return AudioMetadata{}, fmt.Errorf("%w: .mp3 MIME mismatch (%q)", ErrInvalidAudio, detectedMIME)
		}
		return inspectMP3(r)
	case ".wav":
		switch normMIME {
		case "audio/wav", "audio/x-wav", "audio/vnd.wave":
		default:
			return AudioMetadata{}, fmt.Errorf("%w: .wav MIME mismatch (%q)", ErrInvalidAudio, detectedMIME)
		}
		return inspectWAV(r)
	case ".m4a":
		switch normMIME {
		case "audio/mp4", "audio/x-m4a", "video/mp4":
		default:
			return AudioMetadata{}, fmt.Errorf("%w: .m4a MIME mismatch (%q)", ErrInvalidAudio, detectedMIME)
		}
		return inspectM4A(r)
	default:
		return AudioMetadata{}, fmt.Errorf("%w: %s", ErrUnsupportedAudio, ext)
	}
}

func normalizeMIME(raw string) string {
	base, _, _ := strings.Cut(raw, ";")
	return strings.ToLower(strings.TrimSpace(base))
}

func intPtr(v int) *int {
	return &v
}

func inspectWAV(r io.ReadSeeker) (AudioMetadata, error) {
	fileSize, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek wav end: %w", ErrInvalidAudio, err)
	}
	if fileSize < 12 {
		return AudioMetadata{}, fmt.Errorf("%w: wav file too short", ErrInvalidAudio)
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek wav start: %w", ErrInvalidAudio, err)
	}

	var riff [12]byte
	if _, err = io.ReadFull(r, riff[:]); err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: read wav riff header: %w", ErrInvalidAudio, err)
	}
	if string(riff[0:4]) != "RIFF" || string(riff[8:12]) != "WAVE" {
		return AudioMetadata{}, fmt.Errorf("%w: missing RIFF/WAVE header", ErrInvalidAudio)
	}
	riffSize := int64(binary.LittleEndian.Uint32(riff[4:8]))
	if riffSize < 4 || riffSize+8 > fileSize {
		return AudioMetadata{}, fmt.Errorf("%w: invalid RIFF size bounds", ErrInvalidAudio)
	}
	riffEnd := riffSize + 8

	var (
		hasFmt            bool
		hasData           bool
		isUncompressedPCM bool
		byteRate          uint32
		blockAlign        uint16
		dataBytes         int64
		offset            int64 = 12
		chunkHdr          [8]byte
		fmtPayload        [16]byte
	)

	for chunks := 0; chunks < maxWAVChunks && offset+8 <= riffEnd; chunks++ {
		if _, err = r.Seek(offset, io.SeekStart); err != nil {
			return AudioMetadata{}, fmt.Errorf("%w: seek wav chunk: %w", ErrInvalidAudio, err)
		}
		if _, err = io.ReadFull(r, chunkHdr[:]); err != nil {
			return AudioMetadata{}, fmt.Errorf("%w: read wav chunk header: %w", ErrInvalidAudio, err)
		}
		chunkID := string(chunkHdr[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHdr[4:8]))
		payloadStart := offset + 8
		if payloadStart+chunkSize > riffEnd {
			return AudioMetadata{}, fmt.Errorf("%w: wav chunk %q exceeds RIFF container", ErrInvalidAudio, chunkID)
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return AudioMetadata{}, fmt.Errorf("%w: wav fmt chunk too small (%d)", ErrInvalidAudio, chunkSize)
			}
			if _, err = io.ReadFull(r, fmtPayload[:]); err != nil {
				return AudioMetadata{}, fmt.Errorf("%w: read wav fmt chunk: %w", ErrInvalidAudio, err)
			}
			audioFormat := binary.LittleEndian.Uint16(fmtPayload[0:2])
			channels := binary.LittleEndian.Uint16(fmtPayload[2:4])
			sampleRate := binary.LittleEndian.Uint32(fmtPayload[4:8])
			br := binary.LittleEndian.Uint32(fmtPayload[8:12])
			ba := binary.LittleEndian.Uint16(fmtPayload[12:14])
			bitsPerSample := binary.LittleEndian.Uint16(fmtPayload[14:16])
			if channels == 0 || sampleRate == 0 || br == 0 || ba == 0 {
				return AudioMetadata{}, fmt.Errorf("%w: invalid wav fmt parameters", ErrInvalidAudio)
			}
			if audioFormat == 1 || audioFormat == 3 {
				if bitsPerSample == 0 || bitsPerSample%8 != 0 {
					return AudioMetadata{}, fmt.Errorf("%w: invalid wav PCM bitsPerSample (%d)", ErrInvalidAudio, bitsPerSample)
				}
				expectedBlockAlign := uint32(channels) * uint32(bitsPerSample/8)
				if uint32(ba) != expectedBlockAlign || br != sampleRate*expectedBlockAlign {
					return AudioMetadata{}, fmt.Errorf("%w: inconsistent wav PCM blockAlign/byteRate", ErrInvalidAudio)
				}
				isUncompressedPCM = true
			}
			hasFmt = true
			byteRate = br
			blockAlign = ba
		case "data":
			if chunkSize > 0 {
				hasData = true
				dataBytes = chunkSize
			}
		}

		// RIFF chunks are word-aligned (2 bytes).
		padded := chunkSize + (chunkSize & 1)
		nextOffset := payloadStart + padded
		if nextOffset <= offset {
			return AudioMetadata{}, fmt.Errorf("%w: invalid wav chunk size", ErrInvalidAudio)
		}
		offset = nextOffset
		if hasFmt && hasData {
			break
		}
	}

	if !hasFmt || !hasData || byteRate == 0 || dataBytes < int64(blockAlign) {
		return AudioMetadata{}, fmt.Errorf("%w: missing wav fmt or valid audio data", ErrInvalidAudio)
	}
	if isUncompressedPCM && dataBytes%int64(blockAlign) != 0 {
		return AudioMetadata{}, fmt.Errorf("%w: wav PCM data length %d not aligned to blockAlign %d", ErrInvalidAudio, dataBytes, blockAlign)
	}

	secs := int(math.Round(float64(dataBytes) / float64(byteRate)))
	if secs < 1 {
		secs = 1
	}
	return AudioMetadata{
		MIMEType:        "audio/wav",
		DurationSeconds: intPtr(secs),
	}, nil
}

func inspectMP3(r io.ReadSeeker) (AudioMetadata, error) {
	fileSize, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek mp3 end: %w", ErrInvalidAudio, err)
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek mp3 start: %w", ErrInvalidAudio, err)
	}

	skip, id3Err := skipID3v2Offset(r, fileSize)
	if id3Err != nil {
		return AudioMetadata{}, id3Err
	}
	if _, err = r.Seek(skip, io.SeekStart); err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek past ID3v2 header: %w", ErrInvalidAudio, err)
	}

	dec := mp3.NewDecoder(r)
	var (
		frame         mp3.Frame
		validFrames   int
		totalDuration time.Duration
		hitFrameLimit bool
	)

	for i := 0; ; i++ {
		if i >= maxMP3Frames {
			hitFrameLimit = true
			break
		}
		skipped := 0
		err = dec.Decode(&frame, &skipped)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if validFrames == 0 {
				return AudioMetadata{}, fmt.Errorf("%w: decode mp3 frame: %w", ErrInvalidAudio, err)
			}
			// Trailing garbage or truncated ID3v1/tag tail after valid frames ends decoding cleanly.
			break
		}
		if validFrames == 0 && skipped > maxMP3InitialSkip {
			return AudioMetadata{}, fmt.Errorf("%w: excessive non-MPEG prefix before first frame (%d bytes)", ErrInvalidAudio, skipped)
		}
		dur := frame.Duration()
		if dur <= 0 {
			continue
		}
		validFrames++
		totalDuration += dur
	}

	if validFrames == 0 {
		return AudioMetadata{}, fmt.Errorf("%w: no valid MPEG audio frames found", ErrInvalidAudio)
	}

	var durPtr *int
	if !hitFrameLimit && totalDuration > 0 {
		secs := int(math.Round(totalDuration.Seconds()))
		if secs < 1 {
			secs = 1
		}
		durPtr = intPtr(secs)
	}
	return AudioMetadata{
		MIMEType:        "audio/mpeg",
		DurationSeconds: durPtr,
	}, nil
}

func skipID3v2Offset(r io.ReadSeeker, fileSize int64) (int64, error) {
	if fileSize < 10 {
		return 0, nil
	}
	var hdr [10]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, fmt.Errorf("%w: read mp3 header: %w", ErrInvalidAudio, err)
	}
	if string(hdr[0:3]) != "ID3" {
		return 0, nil
	}
	for _, b := range hdr[6:10] {
		if b&0x80 != 0 {
			return 0, fmt.Errorf("%w: malformed ID3v2 size", ErrInvalidAudio)
		}
	}
	tagSize := int64(hdr[6])<<21 | int64(hdr[7])<<14 | int64(hdr[8])<<7 | int64(hdr[9])
	total := 10 + tagSize
	if hdr[5]&0x10 != 0 {
		total += 10
	}
	if total > maxID3v2HeaderBytes || total > fileSize {
		return 0, fmt.Errorf("%w: ID3v2 tag exceeds bounds", ErrInvalidAudio)
	}
	return total, nil
}

type mp4TrackState struct {
	isSound       bool
	hasSampleDesc bool
	hasSamples    bool
	trackSeconds  *int
}

func readBoxHeaderSlice(r io.ReadSeeker, bi mp4.BoxInfo, fileSize int64, buf []byte) (payloadLen int64, err error) {
	if bi.Size > uint64(fileSize) || bi.Offset > uint64(fileSize) || bi.Offset+bi.Size > uint64(fileSize) {
		return 0, fmt.Errorf("%w: MP4 box %s exceeds file bounds", ErrInvalidAudio, bi.Type.String())
	}
	if bi.Size < bi.HeaderSize {
		return 0, fmt.Errorf("%w: malformed MP4 box size", ErrInvalidAudio)
	}
	payloadLen = int64(bi.Size - bi.HeaderSize)
	if payloadLen < int64(len(buf)) {
		return 0, fmt.Errorf("%w: MP4 box %s payload too short (%d)", ErrInvalidAudio, bi.Type.String(), payloadLen)
	}
	if _, seekErr := r.Seek(int64(bi.Offset+bi.HeaderSize), io.SeekStart); seekErr != nil {
		return 0, seekErr
	}
	if _, readErr := io.ReadFull(r, buf); readErr != nil {
		return 0, readErr
	}
	return payloadLen, nil
}

func inspectM4A(r io.ReadSeeker) (AudioMetadata, error) {
	fileSize, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek m4a end: %w", ErrInvalidAudio, err)
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: seek m4a start: %w", ErrInvalidAudio, err)
	}

	var (
		hasMoov              bool
		hasValidAudioTrack   bool
		hasAudioTrackSamples bool
		hasFragmentSamples   bool
		hasMdatPayload       bool
		boxesSeen            int
		movieSeconds         *int
		audioSeconds         *int
	)

	_, err = mp4.ReadBoxStructure(r, func(h *mp4.ReadHandle) (any, error) {
		boxesSeen++
		if boxesSeen > maxMP4Boxes {
			return nil, fmt.Errorf("%w: MP4 box limit exceeded", ErrInvalidAudio)
		}
		if len(h.Path) > maxMP4BoxDepth {
			return nil, fmt.Errorf("%w: MP4 box depth limit exceeded", ErrInvalidAudio)
		}
		if h.BoxInfo.Size > uint64(fileSize) || h.BoxInfo.Offset+h.BoxInfo.Size > uint64(fileSize) {
			return nil, fmt.Errorf("%w: MP4 box %s exceeds file size", ErrInvalidAudio, h.BoxInfo.Type.String())
		}

		switch h.BoxInfo.Type {
		case mp4.BoxTypeMoov():
			hasMoov = true
			if _, expandErr := h.Expand(); expandErr != nil {
				return nil, expandErr
			}
		case mp4.BoxTypeTrak():
			ts := &mp4TrackState{}
			if _, expandErr := h.Expand(ts); expandErr != nil {
				return nil, expandErr
			}
			if ts.isSound && ts.hasSampleDesc {
				hasValidAudioTrack = true
				if ts.hasSamples {
					hasAudioTrackSamples = true
				}
				if audioSeconds == nil && ts.trackSeconds != nil {
					audioSeconds = ts.trackSeconds
				}
			}
		case mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl():
			if _, expandErr := h.Expand(h.Params...); expandErr != nil {
				return nil, expandErr
			}
		case mp4.BoxTypeMoof(), mp4.BoxTypeTraf():
			if _, expandErr := h.Expand(h.Params...); expandErr != nil {
				return nil, expandErr
			}
		case mp4.BoxTypeMvhd():
			if h.BoxInfo.Size > maxMP4MetaBoxBytes {
				return nil, fmt.Errorf("%w: mvhd box exceeds limit", ErrInvalidAudio)
			}
			box, _, readErr := h.ReadPayload()
			if readErr != nil {
				return nil, readErr
			}
			if mvhd, ok := box.(*mp4.Mvhd); ok {
				if secs := mp4DurationSeconds(mvhd.Version, mvhd.Timescale, mvhd.GetDuration()); secs != nil {
					movieSeconds = secs
				}
			}
		case mp4.BoxTypeHdlr():
			if len(h.Params) == 0 {
				return nil, nil
			}
			ts, okTS := h.Params[0].(*mp4TrackState)
			if !okTS {
				return nil, nil
			}
			if h.BoxInfo.Size > maxMP4MetaBoxBytes {
				return nil, fmt.Errorf("%w: hdlr box exceeds limit", ErrInvalidAudio)
			}
			// Read only the fixed 12-byte prefix (version/flags + pre_defined + handler_type) without allocating Name.
			var hdr [12]byte
			if _, readErr := readBoxHeaderSlice(r, h.BoxInfo, fileSize, hdr[:]); readErr != nil {
				return nil, readErr
			}
			if string(hdr[8:12]) == "soun" {
				ts.isSound = true
			}
		case mp4.BoxTypeMdhd():
			if len(h.Params) == 0 {
				return nil, nil
			}
			ts, okTS := h.Params[0].(*mp4TrackState)
			if !okTS {
				return nil, nil
			}
			if h.BoxInfo.Size > maxMP4MetaBoxBytes {
				return nil, fmt.Errorf("%w: mdhd box exceeds limit", ErrInvalidAudio)
			}
			box, _, readErr := h.ReadPayload()
			if readErr != nil {
				return nil, readErr
			}
			if mdhd, ok := box.(*mp4.Mdhd); ok {
				if secs := mp4DurationSeconds(mdhd.Version, mdhd.Timescale, mdhd.GetDuration()); secs != nil {
					ts.trackSeconds = secs
				}
			}
		case mp4.BoxTypeStsd():
			if len(h.Params) == 0 {
				return nil, nil
			}
			ts, okTS := h.Params[0].(*mp4TrackState)
			if !okTS {
				return nil, nil
			}
			if h.BoxInfo.Size > maxMP4MetaBoxBytes {
				return nil, fmt.Errorf("%w: stsd box exceeds limit", ErrInvalidAudio)
			}
			// FullBox header (4) + entry_count (4) + first sample entry size (4) + type (4) = 16 bytes.
			var hdr [16]byte
			payloadLen, readErr := readBoxHeaderSlice(r, h.BoxInfo, fileSize, hdr[:])
			if readErr != nil {
				return nil, readErr
			}
			entryCount := binary.BigEndian.Uint32(hdr[4:8])
			firstEntrySize := int64(binary.BigEndian.Uint32(hdr[8:12]))
			if entryCount >= 1 && firstEntrySize >= 8 && 8+firstEntrySize <= payloadLen {
				ts.hasSampleDesc = true
			}
		case mp4.BoxTypeStsz():
			if len(h.Params) == 0 {
				return nil, nil
			}
			ts, okTS := h.Params[0].(*mp4TrackState)
			if !okTS {
				return nil, nil
			}
			var hdr [12]byte
			payloadLen, readErr := readBoxHeaderSlice(r, h.BoxInfo, fileSize, hdr[:])
			if readErr != nil {
				return nil, readErr
			}
			sampleSize := binary.BigEndian.Uint32(hdr[4:8])
			sampleCount := binary.BigEndian.Uint32(hdr[8:12])
			if sampleSize == 0 && int64(12)+4*int64(sampleCount) > payloadLen {
				return nil, fmt.Errorf("%w: stsz table truncated", ErrInvalidAudio)
			}
			if sampleCount > 0 {
				ts.hasSamples = true
			}
		case mp4.StrToBoxType("stz2"):
			if len(h.Params) == 0 {
				return nil, nil
			}
			ts, okTS := h.Params[0].(*mp4TrackState)
			if !okTS {
				return nil, nil
			}
			var hdr [12]byte
			payloadLen, readErr := readBoxHeaderSlice(r, h.BoxInfo, fileSize, hdr[:])
			if readErr != nil {
				return nil, readErr
			}
			fieldSize := hdr[7]
			sampleCount := binary.BigEndian.Uint32(hdr[8:12])
			if (fieldSize != 4 && fieldSize != 8 && fieldSize != 16) ||
				int64(12)+(int64(sampleCount)*int64(fieldSize)+7)/8 > payloadLen {
				return nil, fmt.Errorf("%w: stz2 table truncated or invalid field size", ErrInvalidAudio)
			}
			if sampleCount > 0 {
				ts.hasSamples = true
			}
		case mp4.BoxTypeTrun():
			var hdr [8]byte
			if _, readErr := readBoxHeaderSlice(r, h.BoxInfo, fileSize, hdr[:]); readErr != nil {
				return nil, readErr
			}
			if binary.BigEndian.Uint32(hdr[4:8]) > 0 {
				hasFragmentSamples = true
			}
		case mp4.BoxTypeMdat():
			if h.BoxInfo.Size > h.BoxInfo.HeaderSize {
				hasMdatPayload = true
			}
		}
		return nil, nil
	})
	if err != nil {
		return AudioMetadata{}, fmt.Errorf("%w: parse m4a: %w", ErrInvalidAudio, err)
	}
	if !hasMoov || !hasValidAudioTrack || !hasMdatPayload || (!hasAudioTrackSamples && !hasFragmentSamples) {
		return AudioMetadata{}, fmt.Errorf("%w: m4a missing playable audio track or samples", ErrInvalidAudio)
	}

	dur := audioSeconds
	if dur == nil {
		dur = movieSeconds
	}
	return AudioMetadata{
		MIMEType:        "audio/mp4",
		DurationSeconds: dur,
	}, nil
}

func mp4DurationSeconds(version uint8, timescale uint32, duration uint64) *int {
	if timescale == 0 || duration == 0 {
		return nil
	}
	if (version == 0 && duration == math.MaxUint32) || (version == 1 && duration == math.MaxUint64) {
		return nil
	}
	secsFloat := math.Round(float64(duration) / float64(timescale))
	if math.IsNaN(secsFloat) || math.IsInf(secsFloat, 0) || secsFloat > float64(math.MaxInt32) {
		return nil
	}
	secs := int(secsFloat)
	if secs < 1 {
		secs = 1
	}
	return intPtr(secs)
}
