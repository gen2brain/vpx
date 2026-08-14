package webp

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The WebP corpora are large and live outside the repository. CONFORMANCE_DIR
// is a colon separated list of them; the one holding a webp-conformance
// checkout is the one these suites read.
func conformanceDirs(t *testing.T) []string {
	t.Helper()

	env := os.Getenv("CONFORMANCE_DIR")
	if env == "" {
		t.Skip("set CONFORMANCE_DIR")
	}

	return strings.Split(env, ":")
}

func conformanceRoot(t *testing.T) string {
	t.Helper()

	for _, dir := range conformanceDirs(t) {
		if _, err := os.Stat(filepath.Join(dir, "webp-conformance", "valid")); err == nil {
			return filepath.Join(dir, "webp-conformance")
		}
	}

	t.Skip("no webp-conformance corpus in CONFORMANCE_DIR")

	return ""
}

// corpusFiles is every still the suites read: the conformance corpus, plus the
// tree tools/gencorpus.sh writes if one of the CONFORMANCE_DIR entries has it.
// The corpus has few lossless, alpha and animated files, so the generated ones
// are where most of the coverage for those comes from.
func corpusFiles(t *testing.T) []string {
	t.Helper()

	names, err := filepath.Glob(filepath.Join(conformanceRoot(t), "valid", "*.webp"))
	if err != nil {
		t.Fatal(err)
	}

	for _, dir := range conformanceDirs(t) {
		more, err := filepath.Glob(filepath.Join(dir, "generated", "*.webp"))
		if err != nil {
			t.Fatal(err)
		}

		names = append(names, more...)
	}

	sort.Strings(names)

	return names
}

// oracle is what webpinfo reports about a file. WEBPINFO_BIN overrides the
// binary; the suites skip when it is not on PATH.
type oracle struct {
	width, height int
	alpha         bool
	animation     bool
	frames        int
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

// webpInfo runs webpinfo and reads back the fields DecodeConfig has to match.
// The canvas size of an extended file wins over the bitstream dimensions,
// which is what libwebp reports too.
func webpInfo(bin, path string) (oracle, error) {
	var o oracle

	out, err := exec.Command(bin, path).Output()
	if err != nil {
		return o, err
	}

	var haveCanvas, haveDim, haveAlpha, haveAnim bool

	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		switch {
		case strings.HasPrefix(line, "Chunk ANMF"):
			o.frames++
		case strings.HasPrefix(line, "Canvas size"):
			if _, err := fmt.Sscanf(line, "Canvas size %d x %d", &o.width, &o.height); err != nil {
				return o, err
			}

			haveCanvas = true
		case strings.HasPrefix(line, "Width:") && !haveCanvas && !haveDim:
			o.width, err = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Width:")))
			if err != nil {
				return o, err
			}
		case strings.HasPrefix(line, "Height:") && !haveCanvas && !haveDim:
			o.height, err = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Height:")))
			if err != nil {
				return o, err
			}

			haveDim = true
		case strings.HasPrefix(line, "Alpha:") && !haveAlpha:
			o.alpha = strings.HasSuffix(line, "1")
			haveAlpha = true
		case strings.HasPrefix(line, "Animation:") && !haveAnim:
			o.animation = strings.HasSuffix(line, "1")
			haveAnim = true
		}
	}

	return o, s.Err()
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

// dwebpPAM decodes a file with libwebp and returns its RGBA samples, read back
// out of the PAM header dwebp writes.
func dwebpPAM(bin, path, out string) ([]byte, int, int, error) {
	if err := exec.Command(bin, "-pam", "-o", out, path).Run(); err != nil {
		return nil, 0, 0, err
	}

	b, err := os.ReadFile(out)
	if err != nil {
		return nil, 0, 0, err
	}

	end := bytes.Index(b, []byte("ENDHDR\n"))
	if end < 0 {
		return nil, 0, 0, fmt.Errorf("no PAM header in %s", out)
	}

	var w, h, depth int

	for line := range strings.SplitSeq(string(b[:end]), "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}

		n, err := strconv.Atoi(value)
		if err != nil {
			continue
		}

		switch key {
		case "WIDTH":
			w = n
		case "HEIGHT":
			h = n
		case "DEPTH":
			depth = n
		}
	}

	if depth != 4 {
		return nil, 0, 0, fmt.Errorf("depth %d, want 4", depth)
	}

	return b[end+len("ENDHDR\n"):], w, h, nil
}

// TestConformanceLossless decodes every VP8L file in the corpus and requires
// the RGBA to match libwebp byte for byte. Set CONFORMANCE_DIR to run it.
func TestConformanceLossless(t *testing.T) {
	bin := dwebpBin(t)

	names := corpusFiles(t)

	const baseline = 34

	out := filepath.Join(t.TempDir(), "ref.pam")

	var passed, failed int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		c, err := parse(memSource(data))
		if err != nil || !c.image.id.is(fccVP8L) || c.animated() {
			continue
		}

		want, w, h, err := dwebpPAM(bin, path, out)
		if err != nil {
			t.Logf("%s: %v", name, err)

			continue
		}

		payload, err := c.payload(c.image)
		if err != nil {
			t.Fatal(err)
		}

		img, err := decodeLossless(payload, Options{})
		if err != nil {
			failed++

			t.Errorf("%s: %v", name, err)

			continue
		}

		got, ok := img.(*image.NRGBA)
		if !ok {
			t.Fatalf("%s: %T, want *image.NRGBA", name, img)
		}

		if got.Rect.Dx() != w || got.Rect.Dy() != h {
			failed++

			t.Errorf("%s: %dx%d, want %dx%d", name, got.Rect.Dx(), got.Rect.Dy(), w, h)

			continue
		}

		if diff := comparePixels(got.Pix, want, w, h); diff != "" {
			failed++

			t.Errorf("%s: %s", name, diff)

			continue
		}

		passed++
	}

	t.Logf("%d/%d lossless files match dwebp", passed, passed+failed)

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}

func comparePixels(got, want []byte, w, h int) string {
	if len(got) < 4*w*h || len(want) < 4*w*h {
		return fmt.Sprintf("%d and %d bytes, want %d", len(got), len(want), 4*w*h)
	}

	for i := range 4 * w * h {
		if got[i] != want[i] {
			return fmt.Sprintf("pixel %d channel %d = %d, want %d", i/4, i%4, got[i], want[i])
		}
	}

	return ""
}

// TestConformanceAlpha decodes every lossy file carrying an ALPH chunk and
// requires the alpha plane to match libwebp byte for byte, which dwebp writes
// as a fourth plane after the YUV ones. Set CONFORMANCE_DIR to run it.
func TestConformanceAlpha(t *testing.T) {
	bin := dwebpBin(t)

	names := corpusFiles(t)

	const baseline = 19

	out := filepath.Join(t.TempDir(), "ref.yuv")

	var passed, failed int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		c, err := parse(memSource(data))
		if err != nil || !c.image.id.is(fccVP8) || !c.alpha.valid() || c.animated() {
			continue
		}

		if err := exec.Command(bin, "-yuv", "-o", out, path).Run(); err != nil {
			continue
		}

		want, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}

		img, err := c.decodeStill(Options{})
		if err != nil {
			failed++

			t.Errorf("%s: %v", name, err)

			continue
		}

		got := img.(*image.NYCbCrA)
		w, h := got.Rect.Dx(), got.Rect.Dy()
		uvW, uvH := (w+1)/2, (h+1)/2

		if len(want) != w*h+2*uvW*uvH+w*h {
			t.Errorf("%s: reference has no alpha plane", name)

			continue
		}

		ref := want[w*h+2*uvW*uvH:]

		if diff := comparePlaneBytes(got.A, got.AStride, ref, w, h); diff != "" {
			failed++

			t.Errorf("%s: A%s", name, diff)

			continue
		}

		passed++
	}

	t.Logf("%d/%d alpha planes match dwebp", passed, passed+failed)

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}

func comparePlaneBytes(got []byte, stride int, want []byte, w, h int) string {
	for y := range h {
		g := got[y*stride : y*stride+w]
		e := want[y*w : y*w+w]

		for x := range w {
			if g[x] != e[x] {
				return fmt.Sprintf("[%d,%d] = %d, want %d", x, y, g[x], e[x])
			}
		}
	}

	return ""
}

// TestConformanceContainer parses every valid file and checks the container
// against webpinfo: dimensions, the alpha and animation features, and the
// frame count. Set CONFORMANCE_DIR to run it.
func TestConformanceContainer(t *testing.T) {
	bin := webpInfoBin(t)
	files := corpusFiles(t)

	const baseline = 278

	var passed int

	for _, path := range files {
		name := filepath.Base(path)

		want, err := webpInfo(bin, path)
		if err != nil {
			t.Errorf("%s: webpinfo: %v", name, err)

			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		c, err := parse(memSource(data))
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}

		got, err := c.features()
		if err != nil {
			t.Errorf("%s: %v", name, err)

			continue
		}

		if got.width != want.width || got.height != want.height {
			t.Errorf("%s: %dx%d, want %dx%d", name, got.width, got.height, want.width, want.height)

			continue
		}

		if got.hasAlpha != want.alpha {
			t.Errorf("%s: alpha %v, want %v", name, got.hasAlpha, want.alpha)

			continue
		}

		if got.hasAnimation != want.animation {
			t.Errorf("%s: animation %v, want %v", name, got.hasAnimation, want.animation)

			continue
		}

		if len(c.frames) != want.frames {
			t.Errorf("%s: %d frames, want %d", name, len(c.frames), want.frames)

			continue
		}

		passed++
	}

	t.Logf("%d/%d files match webpinfo", passed, len(files))

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}

// TestConformanceToRGBA decodes every still with Options{ToRGBA: true} and
// requires it to match dwebp's RGBA, premultiplied as MODE_rgbA does. Set
// CONFORMANCE_DIR to run it.
func TestConformanceToRGBA(t *testing.T) {
	bin := dwebpBin(t)

	names := corpusFiles(t)

	const baseline = 271

	out := filepath.Join(t.TempDir(), "ref.pam")

	var passed, failed int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		c, err := parse(memSource(data))
		if err != nil || c.animated() {
			continue
		}

		want, w, h, err := dwebpPAM(bin, path, out)
		if err != nil {
			continue
		}

		premultiply(want[:4*w*h])

		img, err := c.decodeStill(Options{ToRGBA: true})
		if err != nil {
			failed++

			t.Errorf("%s: %v", name, err)

			continue
		}

		got := img.(*image.RGBA)

		if diff := comparePixels(got.Pix, want, w, h); diff != "" {
			failed++

			t.Errorf("%s: %s", name, diff)

			continue
		}

		passed++
	}

	t.Logf("%d/%d stills match dwebp as RGBA", passed, passed+failed)

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}

// TestConformanceToYCbCr decodes every lossless still with
// Options{ToYCbCr: true} and requires the planes to match dwebp's MODE_YUVA,
// which is the conversion the package this replaces gets. Set CONFORMANCE_DIR
// to run it.
func TestConformanceToYCbCr(t *testing.T) {
	bin := dwebpBin(t)

	names := corpusFiles(t)

	const baseline = 34

	out := filepath.Join(t.TempDir(), "ref.yuv")

	var passed, failed int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		c, err := parse(memSource(data))
		if err != nil || !c.image.id.is(fccVP8L) || c.animated() {
			continue
		}

		if err := exec.Command(bin, "-yuv", "-o", out, path).Run(); err != nil {
			continue
		}

		want, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}

		img, err := c.decodeStill(Options{ToYCbCr: true})
		if err != nil {
			failed++

			t.Errorf("%s: %v", name, err)

			continue
		}

		got := img.(*image.NYCbCrA)
		w, h := got.Rect.Dx(), got.Rect.Dy()
		uvW, uvH := (w+1)/2, (h+1)/2

		diff := comparePlaneBytes(got.Y, got.YStride, want[:w*h], w, h)
		if diff == "" {
			diff = comparePlaneBytes(got.Cb, got.CStride, want[w*h:], uvW, uvH)
		}

		if diff == "" {
			diff = comparePlaneBytes(got.Cr, got.CStride, want[w*h+uvW*uvH:], uvW, uvH)
		}

		if diff == "" && len(want) == w*h+2*uvW*uvH+w*h {
			diff = comparePlaneBytes(got.A, got.AStride, want[w*h+2*uvW*uvH:], w, h)
		}

		if diff != "" {
			failed++

			t.Errorf("%s: %s", name, diff)

			continue
		}

		passed++
	}

	t.Logf("%d/%d lossless stills match dwebp as YUV", passed, passed+failed)

	if passed < baseline {
		t.Errorf("%d files match, baseline is %d", passed, baseline)
	}
}
