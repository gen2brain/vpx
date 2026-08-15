package webp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var goldenDecode = map[string]string{
	"2-color.webp":        "8b3be1f343a72dbedd1e0a743d8c77db6bf02ad94a542cbfa05493e08218cddf",
	"anim-dispose.webp":   "5fb6678860f6d1c18b853c70c0b3542ab465c3c6e2c146d28e695595a6eac6d8",
	"anim-small.webp":     "686210527b22950ed84cd253caf700f2c125104ec92c5ff3a4c2b384c09134c3",
	"anim.webp":           "3e5a49628493b095d640d8ec5f0cda567e545a2067deccd8db5e19632b14a16a",
	"exif.webp":           "03e32d66b2c05372a0a4bbfb4079ae973f987464f2e24d20dfac2b94c2e90f33",
	"lossless_alpha.webp": "5362a1b7a88b6fd03c603330832ac50492e331e3f770699edaa2e8ced5e64a2c",
	"lossy_alpha.webp":    "173d8c9b8076fe50041eba2e36300f4ae2e948fb07b3a46b973308fb3e46441f",
	"palette.webp":        "ae49b084f3fd869ec39dfe10500d9bbd66ba44285e5489a57488c161ae5e8763",
	"simple-gray.webp":    "cb35ab5de1ba9a40cbef0baa1f07c292bf4d6b7a15c9692a95a04cf1c7b294c4",
	"simple-rgb.webp":     "ff12073d52e134d4e4b53299eeb1dcf2553c4fb3354b99af39449f567b7b0eb5",
	"simple.webp":         "510803cfb923adb2cc326c9883470749ef0814c9ded9933b40bf59436d26cd61",
	"simple_xmp.webp":     "510803cfb923adb2cc326c9883470749ef0814c9ded9933b40bf59436d26cd61",
	"test.webp":           "a4069fd66062a6545429ebf7875c37e13ff556e6d72801646fb2693437200df9",
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func TestParse(t *testing.T) {
	tests := []struct {
		file      string
		extended  bool
		id        string
		width     int
		height    int
		alpha     bool
		exif      bool
		xmp       bool
		animation bool
		frames    int
		loopCount int
	}{
		{file: "test.webp", id: fccVP8},
		{file: "simple-rgb.webp", id: fccVP8},
		{file: "simple-gray.webp", id: fccVP8},
		{file: "simple.webp", id: fccVP8L},
		{file: "2-color.webp", id: fccVP8L},
		{file: "exif.webp", extended: true, id: fccVP8, width: 512, height: 256, exif: true},
		{file: "simple_xmp.webp", extended: true, id: fccVP8L, width: 300, height: 300, xmp: true},
		{file: "lossy_alpha.webp", extended: true, id: fccVP8, width: 100, height: 100, alpha: true},
		{file: "anim.webp", extended: true, width: 500, height: 360, alpha: true, animation: true, frames: 17},
		{file: "anim-small.webp", extended: true, width: 16, height: 16, animation: true, frames: 2},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			c, err := parse(memSource(readFile(t, tt.file)))
			if err != nil {
				t.Fatal(err)
			}

			if c.extended != tt.extended {
				t.Errorf("extended = %v, want %v", c.extended, tt.extended)
			}

			if tt.id != "" && !c.image.id.is(tt.id) {
				t.Errorf("image chunk = %q, want %q", c.image.id, tt.id)
			}

			if tt.extended && (c.width != tt.width || c.height != tt.height) {
				t.Errorf("canvas = %dx%d, want %dx%d", c.width, c.height, tt.width, tt.height)
			}

			if c.hasAlpha() != tt.alpha {
				t.Errorf("alpha = %v, want %v", c.hasAlpha(), tt.alpha)
			}

			if c.exif.valid() != tt.exif {
				t.Errorf("exif = %v, want %v", c.exif.valid(), tt.exif)
			}

			if c.xmp.valid() != tt.xmp {
				t.Errorf("xmp = %v, want %v", c.xmp.valid(), tt.xmp)
			}

			if c.animated() != tt.animation {
				t.Errorf("animation = %v, want %v", c.animated(), tt.animation)
			}

			if len(c.frames) != tt.frames {
				t.Errorf("frames = %d, want %d", len(c.frames), tt.frames)
			}

			if tt.animation && c.loopCount != tt.loopCount {
				t.Errorf("loop count = %d, want %d", c.loopCount, tt.loopCount)
			}
		})
	}
}

func TestParseFrames(t *testing.T) {
	c, err := parse(memSource(readFile(t, "anim.webp")))
	if err != nil {
		t.Fatal(err)
	}

	for i, f := range c.frames {
		if f.w != 500 || f.h != 360 || f.x != 0 || f.y != 0 {
			t.Errorf("frame %d = %dx%d at %d,%d, want 500x360 at 0,0", i, f.w, f.h, f.x, f.y)
		}

		if f.duration != 80 {
			t.Errorf("frame %d duration = %d, want 80", i, f.duration)
		}

		if !f.image.id.is(fccVP8) {
			t.Errorf("frame %d payload = %q, want %q", i, f.image.id, fccVP8)
		}
	}

	// webpinfo reports the raw bit, where 1 means do not blend, so the first
	// frame is the one that overwrites the canvas.
	if c.frames[0].blend {
		t.Error("frame 0 blends, want overwrite")
	}

	if !c.frames[1].blend {
		t.Error("frame 1 overwrites, want blend")
	}

	if !c.frames[1].alpha.valid() {
		t.Error("frame 1 has no alpha plane")
	}
}

func TestParseInvalid(t *testing.T) {
	good := readFile(t, "test.webp")

	tests := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"short", good[:8]},
		{"no riff", append([]byte("RIFX"), good[4:]...)},
		{"no webp", append(append([]byte{}, good[:8]...), []byte("WEBQ")...)},
		{"riff size too small", riffSize(good, 3)},
		{"truncated chunk", good[:20]},
		{"header only", good[:12]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parse(memSource(tt.data)); err == nil {
				t.Error("parsed, want error")
			}
		})
	}
}

// TestParseTruncated feeds every prefix of a file, which must be rejected or
// parsed, never panic.
func TestParseTruncated(t *testing.T) {
	for _, name := range []string{"test.webp", "simple.webp", "anim.webp", "lossy_alpha.webp"} {
		b := readFile(t, name)

		for n := range len(b) {
			parse(memSource(b[:n]))
		}
	}
}

// TestParseChunkSizeOverflow gives a chunk a length that runs past the file,
// which must not be read.
func TestParseChunkSizeOverflow(t *testing.T) {
	b := bytes.Clone(readFile(t, "lossy_alpha.webp"))
	binary.LittleEndian.PutUint32(b[26:30], 0xfffffff0)

	if _, err := parse(memSource(b)); err == nil {
		t.Error("parsed, want error")
	}
}

func TestCanvasAreaLimit(t *testing.T) {
	b := bytes.Clone(readFile(t, "lossy_alpha.webp"))

	putUint24(b[24:27], 1<<16-1)
	putUint24(b[27:30], 1<<13-1)

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)

	_, err := Decode(bytes.NewReader(b))

	runtime.ReadMemStats(&after)

	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want %v", err, ErrUnsupported)
	}

	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Errorf("allocated %d bytes, want the canvas never to be reached", n)
	}
}

func riffSize(b []byte, size uint32) []byte {
	out := bytes.Clone(b)
	binary.LittleEndian.PutUint32(out[4:8], size)

	return out
}

func TestDecode(t *testing.T) {
	tests := []struct {
		file          string
		kind          string
		width, height int
	}{
		{"test.webp", "*image.NYCbCrA", 512, 512},
		{"simple-rgb.webp", "*image.NYCbCrA", 100, 100},
		{"simple-gray.webp", "*image.NYCbCrA", 100, 100},
		{"lossy_alpha.webp", "*image.NYCbCrA", 100, 100},
		{"exif.webp", "*image.NYCbCrA", 512, 256},
		{"simple.webp", "*image.NRGBA", 300, 300},
		{"2-color.webp", "*image.NRGBA", 300, 300},
		{"simple_xmp.webp", "*image.NRGBA", 300, 300},
		{"palette.webp", "*image.NRGBA", 61, 37},
		{"lossless_alpha.webp", "*image.NRGBA", 48, 33},
		{"anim.webp", "*image.RGBA", 500, 360},
		{"anim-small.webp", "*image.RGBA", 16, 16},
		{"anim-dispose.webp", "*image.RGBA", 96, 64},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			img, err := Decode(bytes.NewReader(readFile(t, tt.file)))
			if err != nil {
				t.Fatal(err)
			}

			if got := fmt.Sprintf("%T", img); got != tt.kind {
				t.Errorf("type = %s, want %s", got, tt.kind)
			}

			b := img.Bounds()
			if b.Dx() != tt.width || b.Dy() != tt.height {
				t.Errorf("bounds = %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.width, tt.height)
			}
		})
	}
}

func TestDecodeAllFrames(t *testing.T) {
	tests := []struct {
		file      string
		frames    int
		delay     int
		loopCount int
	}{
		{"anim.webp", 17, 80, 0},
		{"anim-small.webp", 2, 100, 0},
		{"anim-dispose.webp", 5, 80, 3},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			ret, err := DecodeAll(bytes.NewReader(readFile(t, tt.file)))
			if err != nil {
				t.Fatal(err)
			}

			if len(ret.Image) != tt.frames || len(ret.Delay) != tt.frames {
				t.Fatalf("%d images and %d delays, want %d of each",
					len(ret.Image), len(ret.Delay), tt.frames)
			}

			for i, d := range ret.Delay {
				if d != tt.delay {
					t.Errorf("delay %d = %d, want %d", i, d, tt.delay)
				}
			}

			if ret.LoopCount != tt.loopCount {
				t.Errorf("loop count = %d, want %d", ret.LoopCount, tt.loopCount)
			}
		})
	}
}

func FuzzDecode(f *testing.F) {
	names, err := filepath.Glob(filepath.Join("testdata", "*.webp"))
	if err != nil {
		f.Fatal(err)
	}

	const maxSeed = 16 << 10

	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}

		if len(b) <= maxSeed {
			f.Add(b)
		}
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		DecodeConfig(bytes.NewReader(b))
		Decode(bytes.NewReader(b))
		DecodeAll(bytes.NewReader(b))
	})
}

func FuzzDecodeVP8L(f *testing.F) {
	names, err := filepath.Glob(filepath.Join("testdata", "*.webp"))
	if err != nil {
		f.Fatal(err)
	}

	for _, name := range names {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}

		c, err := parse(memSource(b))
		if err != nil || !c.image.id.is(fccVP8L) {
			continue
		}

		payload, err := c.payload(c.image)
		if err != nil {
			continue
		}

		f.Add(payload)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		var d losslessDecoder

		decodeVP8L(&d, b)
	})
}

func TestDecodeConfig(t *testing.T) {
	tests := []struct {
		file          string
		width, height int
	}{
		{"test.webp", 512, 512},
		{"simple-rgb.webp", 100, 100},
		{"simple-gray.webp", 100, 100},
		{"simple.webp", 300, 300},
		{"2-color.webp", 300, 300},
		{"exif.webp", 512, 256},
		{"simple_xmp.webp", 300, 300},
		{"lossy_alpha.webp", 100, 100},
		{"anim.webp", 500, 360},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			cfg, err := DecodeConfig(bytes.NewReader(readFile(t, tt.file)))
			if err != nil {
				t.Fatal(err)
			}

			if cfg.Width != tt.width || cfg.Height != tt.height {
				t.Errorf("config = %dx%d, want %dx%d", cfg.Width, cfg.Height, tt.width, tt.height)
			}
		})
	}
}

// countingReader serves a file through the interfaces srcFor looks for, and
// counts the bytes it hands out.
type countingReader struct {
	r    *bytes.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) { return c.r.Read(p) }

func (c *countingReader) Seek(off int64, whence int) (int64, error) {
	return c.r.Seek(off, whence)
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.read += n

	return n, err
}

// TestReadsLazily pins how much of a file each entry point has to touch. A
// seekable reader is addressed by range, so the metadata a decode does not
// need is never read.
func TestReadsLazily(t *testing.T) {
	const xmpSize = 2868

	b := readFile(t, "simple_xmp.webp")

	tests := []struct {
		name string
		max  int
		fn   func(r io.Reader) error
	}{
		{"DecodeConfig", 64, func(r io.Reader) error {
			_, err := DecodeConfig(r)

			return err
		}},
		{"DecodeExif", 1024, func(r io.Reader) error {
			_, err := DecodeExif(r)
			if errors.Is(err, ErrNoExif) {
				return nil
			}

			return err
		}},
		{"Decode", len(b) - xmpSize + 64, func(r io.Reader) error {
			_, err := Decode(r)

			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &countingReader{r: bytes.NewReader(b)}

			if err := tt.fn(c); err != nil {
				t.Fatal(err)
			}

			if c.read > tt.max {
				t.Errorf("read %d of %d bytes, want at most %d", c.read, len(b), tt.max)
			}

			t.Logf("read %d of %d bytes", c.read, len(b))
		})
	}
}

// TestDecodeAllReadsOneFrame checks that Decode stops after the frame it
// returns rather than decoding the whole animation.
func TestDecodeAllReadsOneFrame(t *testing.T) {
	b := readFile(t, "anim.webp")

	one := &countingReader{r: bytes.NewReader(b)}
	if _, err := Decode(one); err != nil {
		t.Fatal(err)
	}

	all := &countingReader{r: bytes.NewReader(b)}
	if _, err := DecodeAll(all); err != nil {
		t.Fatal(err)
	}

	if one.read >= all.read {
		t.Errorf("Decode read %d bytes and DecodeAll %d, want fewer", one.read, all.read)
	}

	t.Logf("Decode read %d of %d bytes, DecodeAll %d", one.read, len(b), all.read)
}

func TestDecodeOutputModes(t *testing.T) {
	files := []string{"test.webp", "lossy_alpha.webp", "simple.webp", "lossless_alpha.webp", "palette.webp"}

	modes := []struct {
		name string
		opts Options
		kind string
	}{
		{"ToRGBA", Options{ToRGBA: true}, "*image.RGBA"},
		{"ToYCbCr", Options{ToYCbCr: true}, "*image.NYCbCrA"},
	}

	for _, m := range modes {
		for _, file := range files {
			t.Run(m.name+"/"+file, func(t *testing.T) {
				b := readFile(t, file)

				img, err := Decode(bytes.NewReader(b), m.opts)
				if err != nil {
					t.Fatal(err)
				}

				if got := fmt.Sprintf("%T", img); got != m.kind {
					t.Errorf("type = %s, want %s", got, m.kind)
				}

				cfg, err := DecodeConfig(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}

				if r := img.Bounds(); r.Dx() != cfg.Width || r.Dy() != cfg.Height {
					t.Errorf("bounds = %v, want %dx%d", r, cfg.Width, cfg.Height)
				}
			})
		}
	}
}

func readFileBytes(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join("testdata", name))
}

// TestDecodeAllocs pins the allocation counts, which the reuse and pooling in
// the decode path exist to keep down. The bounds are ceilings, not exact
// counts: raise one only with a reason.
func TestDecodeAllocs(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector allocates")
	}

	tests := []struct {
		file string
		max  float64
		fn   func(b []byte)
	}{
		{"test.webp", 12, func(b []byte) { Decode(bytes.NewReader(b)) }},
		{"lossy_alpha.webp", 24, func(b []byte) { Decode(bytes.NewReader(b)) }},
		{"simple.webp", 100, func(b []byte) { Decode(bytes.NewReader(b)) }},
		{"test.webp", 6, func(b []byte) { DecodeConfig(bytes.NewReader(b)) }},
		{"anim.webp", 300, func(b []byte) { DecodeAll(bytes.NewReader(b)) }},
		// Two above the serial ceiling: the pipeline's goroutines, when the
		// scheduler does not reuse their gs.
		{"test.webp", 14, func(b []byte) { Decode(bytes.NewReader(b), Options{Threads: 3}) }},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			b := readFile(t, tt.file)

			tt.fn(b)

			n := testing.AllocsPerRun(5, func() { tt.fn(b) })
			if n > tt.max {
				t.Errorf("%v allocations, want at most %v", n, tt.max)
			}

			t.Logf("%v allocations", n)
		})
	}
}

// fileSurvives reports a panic as a message. Any error is fine; a panic on
// untrusted input is not.
func fileSurvives(b []byte) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("panic: %v", r)
		}
	}()

	DecodeConfig(bytes.NewReader(b))
	Decode(bytes.NewReader(b))
	Decode(bytes.NewReader(b), Options{ToRGBA: true})
	Decode(bytes.NewReader(b), Options{ToYCbCr: true, AlphaDither: 100})
	DecodeAll(bytes.NewReader(b))
	DecodeExif(bytes.NewReader(b))

	return ""
}

// TestMalformedFiles truncates and corrupts every bundled file, through every
// entry point. The decoder is allowed to reject any of them and not to panic
// on any of them.
func TestMalformedFiles(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("testdata", "*.webp"))
	if err != nil || len(names) == 0 {
		t.Skip("no bundled files")
	}

	for _, path := range names {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		name := filepath.Base(path)

		for _, frac := range []int{0, 1, 2, 3, 5, 7, 11, 16, 32, 48, 63} {
			n := len(data) * frac / 64

			if msg := fileSurvives(data[:n]); msg != "" {
				t.Errorf("%s truncated to %d/%d: %s", name, n, len(data), msg)
			}
		}

		// The offsets cover the RIFF header, the first chunk header, the
		// bitstream header and a spread of payload.
		for _, off := range []int{0, 4, 8, 12, 16, 20, 23, 29, 37, 101, 409, 1021, 4099} {
			if off >= len(data) {
				continue
			}

			for _, bit := range []uint{0, 3, 7} {
				bad := make([]byte, len(data))
				copy(bad, data)
				bad[off] ^= 1 << bit

				if msg := fileSurvives(bad); msg != "" {
					t.Errorf("%s bit %d of byte %d flipped: %s", name, bit, off, msg)
				}
			}
		}
	}
}

func hashImage(h io.Writer, img image.Image) {
	fmt.Fprintf(h, "%T %v ", img, img.Bounds())

	switch p := img.(type) {
	case *image.NRGBA:
		fmt.Fprintf(h, "%d ", p.Stride)
		h.Write(p.Pix)
	case *image.RGBA:
		fmt.Fprintf(h, "%d ", p.Stride)
		h.Write(p.Pix)
	case *image.Gray:
		fmt.Fprintf(h, "%d ", p.Stride)
		h.Write(p.Pix)
	case *image.YCbCr:
		fmt.Fprintf(h, "%d %d %d ", p.YStride, p.CStride, p.SubsampleRatio)
		h.Write(p.Y)
		h.Write(p.Cb)
		h.Write(p.Cr)
	case *image.NYCbCrA:
		fmt.Fprintf(h, "%d %d %d %d ", p.YStride, p.CStride, p.SubsampleRatio, p.AStride)
		h.Write(p.Y)
		h.Write(p.Cb)
		h.Write(p.Cr)
		h.Write(p.A)
	default:
		fmt.Fprintf(h, "unhandled")
	}
}

func goldenHash(data []byte) string {
	return goldenHashThreads(data, 0)
}

func goldenHashThreads(data []byte, threads int) string {
	h := sha256.New()

	for _, o := range []Options{{}, {ToRGBA: true}, {ToYCbCr: true}} {
		o.Threads = threads

		img, err := Decode(bytes.NewReader(data), o)
		if err != nil {
			fmt.Fprintf(h, "decode %v ", err)

			continue
		}

		hashImage(h, img)
	}

	all, err := DecodeAll(bytes.NewReader(data), Options{Threads: threads})
	if err != nil {
		fmt.Fprintf(h, "all %v ", err)

		return hex.EncodeToString(h.Sum(nil))
	}

	fmt.Fprintf(h, "%d %d ", len(all.Image), all.LoopCount)

	for i, img := range all.Image {
		if i < len(all.Delay) {
			fmt.Fprintf(h, "%d ", all.Delay[i])
		}

		hashImage(h, img)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// TestGoldenDecode pins the decoded output of every bundled file so the noasm
// and kernel builds are provably identical, not merely both right against
// libwebp. Run it under -tags noasm as well.
func TestGoldenDecode(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.webp"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no testdata: %v", err)
	}

	seen := make(map[string]bool, len(files))

	for _, path := range files {
		name := filepath.Base(path)
		seen[name] = true

		got := goldenHash(readFile(t, name))

		want, ok := goldenDecode[name]
		if !ok {
			t.Errorf("%s: no golden, computed %q", name, got)

			continue
		}

		if got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}

	for name := range goldenDecode {
		if !seen[name] {
			t.Errorf("%s: golden for a file that is gone", name)
		}
	}
}

// TestGoldenCorpus does the same over the external corpus, against a listing
// written beside it on first run. Run once per build to compare them.
func TestGoldenCorpus(t *testing.T) {
	dir := os.Getenv("CONFORMANCE_DIR")
	if dir == "" {
		t.Skip("set CONFORMANCE_DIR")
	}

	var files []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".webp") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sort.Strings(files)

	var b strings.Builder

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(&b, "%s %s\n", goldenHash(data), filepath.ToSlash(rel))
	}

	golden := filepath.Join(dir, "golden.txt")

	want, err := os.ReadFile(golden)
	if os.IsNotExist(err) {
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}

		t.Logf("wrote %s for %d files; rerun under the other build to compare", golden, len(files))

		return
	}

	if err != nil {
		t.Fatal(err)
	}

	if string(want) == b.String() {
		t.Logf("%d corpus files match %s", len(files), golden)

		return
	}

	wantLines := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
	gotLines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")

	if len(wantLines) != len(gotLines) {
		t.Fatalf("%s has %d entries, corpus has %d; delete it to regenerate",
			golden, len(wantLines), len(gotLines))
	}

	for i := range gotLines {
		if gotLines[i] != wantLines[i] {
			t.Errorf("golden mismatch:\n got %s\nwant %s", gotLines[i], wantLines[i])
		}
	}
}

// TestThreadsMatchSerial requires every thread count to decode to the same
// bytes as Threads: 1, over the bundled files and the corpus when it is set.
func TestThreadsMatchSerial(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.webp"))
	if err != nil {
		t.Fatal(err)
	}

	if dir := os.Getenv("CONFORMANCE_DIR"); dir != "" {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".webp") {
				files = append(files, path)
			}

			return nil
		})
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		want := goldenHashThreads(data, 1)

		for _, n := range []int{0, 2, 3, 4, 16} {
			if got := goldenHashThreads(data, n); got != want {
				t.Fatalf("%s: Threads %d gives %s, serial gives %s", filepath.Base(path), n, got, want)
			}
		}
	}

	t.Logf("%d files decode identically at every thread count", len(files))
}
