package echoprint

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

const (
	// 60 seconds worth of time offsets (60*1000 / 23.2)
	fpSixtySecOffset = 2586

	mediumQualityThreshold = 256
	lowQualityThreshold    = 128

	qualityHigh   = "high"
	qualityMedium = "medium"
	qualityLow    = "low"
)

type metadata struct {
	TrackID  uint32  `json:"track_id"`
	UPC      string  `json:"upc"`
	ISRC     string  `json:"isrc"`
	Version  float64 `json:"version"`
	Filename string  `json:"filename"`
	Bitrate  float64 `json:"bitrate"`
	Duration float64 `json:"duration"`
}

// CodegenFp represents a parsed json fingerprint generated by codegen
type CodegenFp struct {
	Meta metadata `json:"metadata"`
	Code string   `json:"code"`
}

// Fingerprint contains the uncompressed and decoded codegen fingerprint string
// split into Codes and Times as integer arrays
type Fingerprint struct {
	Codes   []uint32
	Times   []uint32
	Meta    metadata
	clamped bool
}

// func (fp *Fingerprint) NewClamped() *Fingerprint {
// 	clampedFp := &Fingerprint{Version: fp.Version, clamped: true}
//
// 	// if we use the codegen on a file with start/stop times, the first timestamp
// 	// is ~= the start time given. There might be a (slightly) earlier timestamp
// 	// in another band, but this is good enough
// 	clampDuration := fpSixtySecOffset + fp.Times[0]
// 	for i, time := range fp.Times {
// 		if time < clampDuration {
// 			clampedFp.Codes = append(clampedFp.Codes, fp.Codes[i])
// 			clampedFp.Times = append(clampedFp.Times, fp.Times[i])
// 		}
// 	}
//
// 	return clampedFp
// }

// NewClamped returns a new Fingerprint limited to 60 seconds
// this is the "wrong" version introduced by the node port of echonest, this
// clamps the codes to only the first band of the fingerprint, we need 180+
// seconds worth of codes to be accurate in a large DB
func (fp *Fingerprint) NewClamped() *Fingerprint {
	clampedFp := &Fingerprint{Codes: fp.Codes, Times: fp.Times, Meta: fp.Meta, clamped: true}

	glog.V(3).Infof("%d Fingerprint Codes Before Clamping", len(fp.Codes))

	// if we use the codegen on a file with start/stop times, the first timestamp
	// is ~= the start time given. There might be a (slightly) earlier timestamp
	// in another band, but this is good enough
	clampDuration := fpSixtySecOffset*3 + fp.Times[0]
	for i, time := range fp.Times {
		if time > clampDuration {
			clampedFp.Codes = fp.Codes[:i]
			clampedFp.Times = fp.Times[:i]
			break
		}
	}

	glog.V(3).Infof("%d Fingerprint Codes After Clamping", len(clampedFp.Codes))
	return clampedFp
}

// Quality returns a string representation of the audio quality of the fingerprint
// based on the bitrate provided in the codegen metadata
func (fp *Fingerprint) Quality() string {
	// if no bitrate is defined we assume high quality for the sake of testing
	if fp.Meta.Bitrate == 0 {
		return qualityHigh
	}
	switch {
	case fp.Meta.Bitrate >= mediumQualityThreshold:
		return qualityHigh
	case fp.Meta.Bitrate >= lowQualityThreshold:
		return qualityMedium
	default:
		return qualityLow
	}
}

func (fp *Fingerprint) isMediumQuality() bool {
	return fp.Meta.Bitrate > mediumQualityThreshold
}

// NewFingerprint decodes the codegen data and splits the audio fingerprint into a pair of
// Code/Time integer arrays of equal size
func NewFingerprint(codegenFp *CodegenFp) (*Fingerprint, error) {
	fp := &Fingerprint{Meta: codegenFp.Meta}
	var err error

	inflated, err := inflate(codegenFp.Code)
	if err != nil {
		glog.Error(err)
		return nil, err
	}

	fp.Codes, fp.Times, err = decode(inflated)
	return fp, err
}

// inflate decodes and decompresses the data generated by codegen
func inflate(data string) (string, error) {
	t := trackTime("inflate")
	defer t.finish()

	// fix some url-safeness that codegen does...
	var fixed string
	fixed = strings.Replace(data, "-", "+", -1)
	fixed = strings.Replace(fixed, "_", "/", -1)

	decoded, err := base64.StdEncoding.DecodeString(fixed)
	if err != nil {
		glog.Error(err)
		return "", err
	}

	r, err := zlib.NewReader(bytes.NewReader(decoded))
	if err != nil {
		glog.Error(err)
		return "", err
	}
	defer r.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	inflated := buf.String()

	return inflated, nil
}

// decode takes an uncompressed code string consisting of zero-padded
// fixed-width sorted hex integers (time values followed by hash codes) and
// converts it to a pair of uint code/time arrays
func decode(fp string) ([]uint32, []uint32, error) {
	t := trackTime("decode")
	defer t.finish()

	// 5 hex bytes for hash, 5 hex bytes for time (40 bits per tuple)
	tupleCount := len(fp) / 5
	length := tupleCount / 2
	codes := make([]uint32, length)
	times := make([]uint32, length)

	var offset int
	var conv uint64
	var err error
	var i int

	// first half of string (time values)
	for ; i < length; i++ {
		offset = i * 5
		conv, err = strconv.ParseUint(fp[offset:offset+5], 16, 32)
		if err != nil {
			return nil, nil, err
		}
		times[i] = uint32(conv)
	}

	// second half of string (code values)
	for ; i < tupleCount; i++ {
		offset = i * 5
		conv, err = strconv.ParseUint(fp[offset:offset+5], 16, 32)
		if err != nil {
			return nil, nil, err
		}
		codes[i-length] = uint32(conv)
	}

	return codes, times, nil
}
