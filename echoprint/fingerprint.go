package echoprint

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"strconv"
	"strings"
)

// Fingerprint contains the uncompressed and decoded codegen fingerprint string
// split into Codes and Times uint arrays
type Fingerprint struct {
	Version float64
	Codes   []uint32
	Times   []uint32
	clamped bool
}

const (
	// 60 seconds worth of time offsets (60*1000 / 23.2)
	fpSixtySecOffset = 2586
)

// NewClamped returns a new Fingerprint limited to 60 seconds
func (fp *Fingerprint) NewClamped() *Fingerprint {
	clampedFp := &Fingerprint{Version: fp.Version, clamped: true}

	// if we use the codegen on a file with start/stop times, the first timestamp
	// is ~= the start time given. There might be a (slightly) earlier timestamp
	// in another band, but this is good enough
	clampDuration := fpSixtySecOffset + fp.Times[0]
	for i, time := range fp.Times {
		if time < clampDuration {
			clampedFp.Codes = append(clampedFp.Codes, fp.Codes[i])
			clampedFp.Times = append(clampedFp.Times, fp.Times[i])
		}
	}

	return clampedFp
}

// NewFingerprint takes a compressed and encoded fingerprint string generated by codegen
// and returns a new Fingerprint containing the Code/Time pairs split into arrays
func NewFingerprint(codegenData string, version float64) (*Fingerprint, error) {
	fp := &Fingerprint{Version: version}
	var err error

	inflated, err := inflate(codegenData)
	if err != nil {
		return nil, err
	}

	fp.Codes, fp.Times, err = decode(inflated)
	return fp, err
}

// inflate decodes and decompresses the data generated by codegen
func inflate(data string) (string, error) {
	t := trackTime("inflate")
	defer t.finish(false)

	// fix some url-safeness that codegen does...
	var fixed string
	fixed = strings.Replace(data, "-", "+", -1)
	fixed = strings.Replace(fixed, "_", "/", -1)

	decoded, err := base64.StdEncoding.DecodeString(fixed)
	if err != nil {
		return "", err
	}

	r, err := zlib.NewReader(bytes.NewReader(decoded))
	defer r.Close()
	if err != nil {
		return "", err
	}

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
	defer t.finish(false)

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
