package vp8

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The corpora are large and live outside the repository. CONFORMANCE_DIR is a
// colon separated list of them.
func corpusFiles(t *testing.T) []string {
	t.Helper()

	names, err := filepath.Glob(filepath.Join(conformanceRoot(t), "valid", "*.webp"))
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range strings.Split(os.Getenv("CONFORMANCE_DIR"), ":") {
		more, err := filepath.Glob(filepath.Join(dir, "generated", "*.webp"))
		if err != nil {
			t.Fatal(err)
		}

		names = append(names, more...)
	}

	sort.Strings(names)

	return names
}

func conformanceRoot(t *testing.T) string {
	t.Helper()

	env := os.Getenv("CONFORMANCE_DIR")
	if env == "" {
		t.Skip("set CONFORMANCE_DIR")
	}

	for _, dir := range strings.Split(env, ":") {
		root := filepath.Join(dir, "webp-conformance")
		if _, err := os.Stat(filepath.Join(root, "valid")); err == nil {
			return root
		}
	}

	t.Skip("no webp-conformance corpus in CONFORMANCE_DIR")

	return ""
}

func webpInfoBin(t *testing.T) string {
	t.Helper()

	name := os.Getenv("WEBPINFO_BIN")
	if name == "" {
		name = "webpinfo"
	}

	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s on PATH", name)
	}

	return path
}

// lossyPayload returns the first VP8 chunk of a WebP file, and reports whether
// the file has one at all. The container is walked here rather than through
// the webp package, which imports this one.
func lossyPayload(data []byte) ([]byte, bool) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, false
	}

	b := data[12:]

	for len(b) >= 8 {
		id := string(b[:4])
		size := binary.LittleEndian.Uint32(b[4:8])

		if uint64(size) > uint64(len(b)-8) {
			return nil, false
		}

		payload := b[8 : 8+size]

		switch id {
		case "VP8 ":
			return payload, true
		case "ANMF":
			b = payload

			continue
		}

		b = b[8+size+size&1:]
	}

	return nil, false
}

// bitstream is the field dump webpinfo prints for a lossy frame, keyed by the
// label it uses.
type bitstream map[string]string

func (b bitstream) num(t *testing.T, key string) int {
	t.Helper()

	v, ok := b[key]
	if !ok {
		t.Fatalf("no %q in dump", key)
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("%q = %q: %v", key, v, err)
	}

	return n
}

func (b bitstream) list(key string) []int {
	var out []int

	for _, f := range strings.Fields(b[key]) {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil
		}

		out = append(out, n)
	}

	return out
}

// webpInfoBitstream runs webpinfo and returns the fields of the first lossy
// frame it dumps.
func webpInfoBitstream(bin, path string) (bitstream, error) {
	out, err := exec.Command(bin, "-bitstream_info", path).Output()
	if err != nil {
		return nil, err
	}

	fields := bitstream{}
	started := false

	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		if strings.HasPrefix(line, "Parsing lossy bitstream") {
			if started {
				break
			}

			started = true

			continue
		}

		if !started {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			break
		}

		fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return fields, s.Err()
}

// TestConformanceHeader parses the frame header of every lossy file in the
// corpus and compares each field against webpinfo. Set CONFORMANCE_DIR to run
// it.
func TestConformanceHeader(t *testing.T) {
	bin := webpInfoBin(t)

	names := corpusFiles(t)

	const baseline = 237

	var passed, skipped int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		payload, ok := lossyPayload(data)
		if !ok {
			skipped++

			continue
		}

		want, err := webpInfoBitstream(bin, path)
		if err != nil || len(want) == 0 {
			skipped++

			continue
		}

		var d Decoder

		if err := d.parseHeader(payload); err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}

		if diff := compareHeader(t, &d, want); diff != "" {
			t.Errorf("%s: %s", name, diff)

			continue
		}

		passed++
	}

	t.Logf("%d/%d lossy files match webpinfo, %d skipped", passed, len(names)-skipped, skipped)

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}

func dwebpBin(t *testing.T) string {
	t.Helper()

	name := os.Getenv("DWEBP_BIN")
	if name == "" {
		name = "dwebp"
	}

	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s on PATH", name)
	}

	return path
}

// dwebpYUV decodes a file with libwebp and returns its planes in the flat
// layout dwebp writes: Y, then U, then V, each cropped to the visible size.
func dwebpYUV(bin, path, out string) ([]byte, error) {
	if err := exec.Command(bin, "-yuv", "-o", out, path).Run(); err != nil {
		return nil, err
	}

	return os.ReadFile(out)
}

// comparePlanes checks a decoded plane against the reference, row by row over
// the visible region, and reports the first sample that differs.
func comparePlanes(name string, got []byte, stride int, want []byte, w, h int) string {
	for y := range h {
		g := got[y*stride : y*stride+w]
		e := want[y*w : y*w+w]

		for x := range w {
			if g[x] != e[x] {
				return fmt.Sprintf("%s[%d,%d] = %d, want %d", name, x, y, g[x], e[x])
			}
		}
	}

	return ""
}

// TestConformanceDecode decodes every lossy file in the corpus and requires
// the planes to match libwebp byte for byte. Set CONFORMANCE_DIR to run it.
func TestConformanceDecode(t *testing.T) {
	bin := dwebpBin(t)

	names := corpusFiles(t)

	const baseline = 237

	out := filepath.Join(t.TempDir(), "ref.yuv")

	var passed, failed, skipped int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		payload, ok := lossyPayload(data)
		if !ok {
			skipped++

			continue
		}

		want, err := dwebpYUV(bin, path, out)
		if err != nil {
			skipped++

			continue
		}

		var d Decoder

		pic, err := d.DecodeFrame(payload)
		if err != nil {
			failed++

			if failed <= 3 {
				t.Logf("%s: %v", name, err)
			}

			continue
		}

		w, h := pic.Width, pic.Height
		uvW, uvH := (w+1)/2, (h+1)/2

		// A file with alpha gets a fourth plane appended, which this suite
		// does not decode.
		if size := w*h + 2*uvW*uvH; len(want) != size && len(want) != size+w*h {
			t.Fatalf("%s: reference is %d bytes, want %d", name, len(want), size)
		}

		diff := comparePlanes("Y", pic.Y, pic.YStride, want[:w*h], w, h)
		if diff == "" {
			diff = comparePlanes("U", pic.U, pic.UVStride, want[w*h:w*h+uvW*uvH], uvW, uvH)
		}

		if diff == "" {
			diff = comparePlanes("V", pic.V, pic.UVStride, want[w*h+uvW*uvH:w*h+2*uvW*uvH], uvW, uvH)
		}

		if diff != "" {
			failed++

			if failed <= 3 || os.Getenv("CONFORMANCE_VERBOSE") != "" {
				t.Logf("%s: %s", name, diff)
			}

			continue
		}

		passed++
	}

	t.Logf("%d/%d lossy files match dwebp, %d failed, %d skipped", passed, passed+failed, failed, skipped)

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}

func compareHeader(t *testing.T, d *Decoder, want bitstream) string {
	t.Helper()

	type check struct {
		key string
		got int
	}

	checks := []check{
		{"Profile", d.hdr.Profile},
		{"Part. 0 length", d.hdr.PartSize},
		{"Width", d.hdr.Width},
		{"X scale", d.hdr.XScale},
		{"Height", d.hdr.Height},
		{"Y scale", d.hdr.YScale},
		{"Color space", d.colorSpace},
		{"Clamp type", d.clampType},
		{"Use segment", boolInt(d.seg.enabled)},
		{"Simple filter", boolInt(d.filter.simple)},
		{"Level", d.filter.level},
		{"Sharpness", d.filter.sharpness},
		{"Use lf delta", boolInt(d.filter.useDelta)},
		{"Total partitions", d.numParts},
		{"Base Q", d.quant.baseQ},
		{"DQ Y1 DC", d.quant.y1DC},
		{"DQ Y2 DC", d.quant.y2DC},
		{"DQ Y2 AC", d.quant.y2AC},
		{"DQ UV DC", d.quant.uvDC},
		{"DQ UV AC", d.quant.uvAC},
	}

	if d.seg.enabled {
		checks = append(checks,
			check{"Update map", boolInt(d.seg.updateMap)},
			check{"Absolute delta", boolInt(d.seg.absoluteDelta)},
		)
	}

	for _, c := range checks {
		if _, ok := want[c.key]; !ok {
			continue
		}

		if n := want.num(t, c.key); n != c.got {
			return fmt.Sprintf("%s = %d, want %d", c.key, c.got, n)
		}
	}

	if v, ok := want["Key frame"]; ok && (v == "Yes") != d.hdr.KeyFrame {
		return fmt.Sprintf("key frame = %v, want %q", d.hdr.KeyFrame, v)
	}

	for _, l := range []struct {
		key string
		got []int
	}{
		{"Quantizer", ints(d.seg.quantizer[:])},
		{"Filter strength", ints(d.seg.filterStrength[:])},
		{"Prob segment", uints(d.proba.segments[:])},
	} {
		if _, ok := want[l.key]; !ok {
			continue
		}

		if w := want.list(l.key); !equal(w, l.got) {
			return fmt.Sprintf("%s = %v, want %v", l.key, l.got, w)
		}
	}

	for i := 1; i < d.numParts; i++ {
		key := fmt.Sprintf("Part. %d length", i)

		if _, ok := want[key]; !ok {
			continue
		}

		if n := want.num(t, key); n != len(d.parts[i-1].buf) {
			return fmt.Sprintf("%s = %d, want %d", key, len(d.parts[i-1].buf), n)
		}
	}

	return ""
}

func boolInt(b bool) int {
	if b {
		return 1
	}

	return 0
}

func ints(v []int8) []int {
	out := make([]int, len(v))
	for i, x := range v {
		out[i] = int(x)
	}

	return out
}

func uints(v []uint8) []int {
	out := make([]int, len(v))
	for i, x := range v {
		out[i] = int(x)
	}

	return out
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func vpxdecBin(t *testing.T) string {
	t.Helper()

	name := os.Getenv("VPXDEC_BIN")
	if name == "" {
		name = "vpxdec"
	}

	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("no %s on PATH", name)
	}

	return path
}

func ivfFrames(b []byte) ([][]byte, error) {
	if len(b) < 32 || string(b[:4]) != "DKIF" {
		return nil, fmt.Errorf("not an IVF file")
	}

	var frames [][]byte

	for off := int(binary.LittleEndian.Uint16(b[6:8])); off < len(b); {
		if off+12 > len(b) {
			return nil, fmt.Errorf("truncated frame header at %d", off)
		}

		n := int(binary.LittleEndian.Uint32(b[off : off+4]))
		off += 12

		if off+n > len(b) {
			return nil, fmt.Errorf("truncated frame at %d", off)
		}

		frames = append(frames, b[off:off+n])
		off += n
	}

	return frames, nil
}

// decodeIVF returns the md5 of every shown frame's I420 planes, which is what
// vpxdec --i420 --md5 hashes.
func decodeIVF(data []byte) (string, int, error) {
	frames, err := ivfFrames(data)
	if err != nil {
		return "", 0, err
	}

	var d Decoder

	h := md5.New()
	shown := 0

	for i, frame := range frames {
		pic, err := d.DecodeFrame(frame)
		if err != nil {
			return "", i, err
		}

		if pic == nil {
			continue
		}

		shown++

		w, hgt := pic.Width, pic.Height

		for y := range hgt {
			h.Write(pic.Y[y*pic.YStride : y*pic.YStride+w])
		}

		cw, ch := (w+1)/2, (hgt+1)/2

		for y := range ch {
			h.Write(pic.U[y*pic.UVStride : y*pic.UVStride+cw])
		}

		for y := range ch {
			h.Write(pic.V[y*pic.UVStride : y*pic.UVStride+cw])
		}
	}

	return hex.EncodeToString(h.Sum(nil)), shown, nil
}

// TestConformanceVideo decodes the VP8 test vectors, which are the streams the
// format is defined by, and requires the md5 of every frame to match what
// vpxdec produces. Set CONFORMANCE_DIR to run it.
func TestConformanceVideo(t *testing.T) {
	bin := vpxdecBin(t)

	var names []string

	for _, dir := range strings.Split(os.Getenv("CONFORMANCE_DIR"), ":") {
		more, err := filepath.Glob(filepath.Join(dir, "VP8-TEST-VECTORS", "*", "*.ivf"))
		if err != nil {
			t.Fatal(err)
		}

		names = append(names, more...)
	}

	if len(names) == 0 {
		t.Skip("no VP8-TEST-VECTORS in CONFORMANCE_DIR")
	}

	sort.Strings(names)

	const baseline = 61

	var passed, failed int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		out, err := exec.Command(bin, "--i420", "--md5", path).Output()
		if err != nil {
			t.Logf("%s: vpxdec: %v", name, err)

			continue
		}

		want, _, _ := strings.Cut(strings.TrimSpace(string(out)), " ")

		got, frame, err := decodeIVF(data)
		if err != nil {
			failed++

			t.Errorf("%s: frame %d: %v", name, frame, err)

			continue
		}

		if got != want {
			failed++

			t.Errorf("%s: md5 %s, want %s", name, got, want)

			continue
		}

		passed++
	}

	t.Logf("%d/%d streams match vpxdec, %d failed", passed, len(names), failed)

	if passed < baseline {
		t.Errorf("%d streams match, baseline is %d", passed, baseline)
	}
}

// BenchmarkDecodeVideo needs the test vectors, which live outside the
// repository behind CONFORMANCE_DIR.
func BenchmarkDecodeVideo(b *testing.B) {
	var path string

	for _, dir := range strings.Split(os.Getenv("CONFORMANCE_DIR"), ":") {
		names, _ := filepath.Glob(filepath.Join(dir, "VP8-TEST-VECTORS", "*", "*-001.ivf"))
		if len(names) > 0 {
			path = names[0]

			break
		}
	}

	if path == "" {
		b.Skip("no VP8-TEST-VECTORS in CONFORMANCE_DIR")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}

	frames, err := ivfFrames(data)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	for _, n := range []int{1, 0} {
		name := "threaded"
		if n == 1 {
			name = "serial"
		}

		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()

			for b.Loop() {
				var d Decoder

				d.Threads = n

				for _, frame := range frames {
					if _, err := d.DecodeFrame(frame); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
