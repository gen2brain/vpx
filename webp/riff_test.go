package webp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
