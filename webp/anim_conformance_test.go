package webp

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func animRefBin(t *testing.T) string {
	t.Helper()

	name := os.Getenv("ANIMREF_BIN")
	if name == "" {
		t.Skip("set ANIMREF_BIN to a libwebp animation dumper")
	}

	return name
}

type animRef struct {
	w, h      int
	loopCount int
	delay     []int
	frames    [][]byte
}

func readAnimRef(path string) (*animRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)

	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fields := strings.Fields(line)
	if len(fields) != 4 {
		return nil, fmt.Errorf("bad header %q", line)
	}

	var a animRef
	var count int

	for i, dst := range []*int{&a.w, &a.h, &count, &a.loopCount} {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			return nil, err
		}

		*dst = n
	}

	for range count {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		d, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			return nil, err
		}

		px := make([]byte, 4*a.w*a.h)
		if _, err := io.ReadFull(r, px); err != nil {
			return nil, err
		}

		a.delay = append(a.delay, d)
		a.frames = append(a.frames, px)
	}

	return &a, nil
}

// TestConformanceAnimation composites every animation in the corpus and
// requires each frame to match libwebp byte for byte. Set CONFORMANCE_DIR and
// ANIMREF_BIN to run it.
func TestConformanceAnimation(t *testing.T) {
	bin := animRefBin(t)

	names := corpusFiles(t)

	const baseline = 7

	out := filepath.Join(t.TempDir(), "ref.bin")

	var passed, failed int

	for _, path := range names {
		name := filepath.Base(path)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		c, err := parse(memSource(data))
		if err != nil || !c.animated() {
			continue
		}

		if err := exec.Command(bin, path, out).Run(); err != nil {
			t.Logf("%s: %v", name, err)

			continue
		}

		want, err := readAnimRef(out)
		if err != nil {
			t.Fatal(err)
		}

		got, err := DecodeAll(bytes.NewReader(data))
		if err != nil {
			failed++

			t.Errorf("%s: %v", name, err)

			continue
		}

		if len(got.Image) != len(want.frames) {
			failed++

			t.Errorf("%s: %d frames, want %d", name, len(got.Image), len(want.frames))

			continue
		}

		if got.LoopCount != want.loopCount {
			t.Errorf("%s: loop count %d, want %d", name, got.LoopCount, want.loopCount)
		}

		bad := ""

		for i := range got.Image {
			if got.Delay[i] != want.delay[i] {
				bad = fmt.Sprintf("frame %d delay %d, want %d", i, got.Delay[i], want.delay[i])

				break
			}

			img := got.Image[i].(*image.RGBA)

			if d := comparePixels(img.Pix, want.frames[i], want.w, want.h); d != "" {
				bad = fmt.Sprintf("frame %d %s", i, d)

				break
			}
		}

		if bad != "" {
			failed++

			t.Errorf("%s: %s", name, bad)

			continue
		}

		passed++
	}

	t.Logf("%d/%d animations match libwebp", passed, passed+failed)

	if passed < baseline {
		t.Errorf("%d animations match, baseline is %d", passed, baseline)
	}
}
