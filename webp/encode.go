package webp

import (
	"image"
	"io"
)

func encode(w io.Writer, m image.Image, o Options) error {
	return ErrUnsupported
}

func encodeAll(w io.Writer, anim *WEBP, o Options) error {
	return ErrUnsupported
}
