// Package apng encodes animated PNG files using only the standard library.
//
// An APNG is a PNG stream plus three extra chunk types: acTL announces the
// animation, fcTL describes a frame and fdAT carries its image data. The
// first frame doubles as the static fallback image, so decoders that do not
// understand APNG still show it.
package apng

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"io"
	"time"
)

var signature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// Encode writes frames as an endlessly looping APNG with the given delay
// between frames. Every frame is drawn at full size with source blending, so
// all frames must have the same bounds.
func Encode(w io.Writer, frames []image.Image, delay time.Duration) error {
	if len(frames) < 2 {
		return errors.New("apng: need at least two frames")
	}
	first := frames[0].Bounds()
	width, height := first.Dx(), first.Dy()
	if width <= 0 || height <= 0 {
		return errors.New("apng: empty frame")
	}
	for i, f := range frames {
		if b := f.Bounds(); b.Dx() != width || b.Dy() != height {
			return fmt.Errorf("apng: frame %d is %dx%d, want %dx%d",
				i, b.Dx(), b.Dy(), width, height)
		}
	}

	if _, err := w.Write(signature); err != nil {
		return err
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(height))
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // color type: truecolor with alpha
	// Bytes 10-12 (compression, filter, interlace) stay zero.
	if err := writeChunk(w, "IHDR", ihdr); err != nil {
		return err
	}
	actl := make([]byte, 8)
	binary.BigEndian.PutUint32(actl[0:], uint32(len(frames)))
	binary.BigEndian.PutUint32(actl[4:], 0) // loop forever
	if err := writeChunk(w, "acTL", actl); err != nil {
		return err
	}

	delayNum, delayDen := delayFraction(delay)
	seq := uint32(0)
	for i, f := range frames {
		data, err := frameData(f, width, height)
		if err != nil {
			return err
		}
		fctl := make([]byte, 26)
		binary.BigEndian.PutUint32(fctl[0:], seq)
		seq++
		binary.BigEndian.PutUint32(fctl[4:], uint32(width))
		binary.BigEndian.PutUint32(fctl[8:], uint32(height))
		// fctl[12:20]: x_offset and y_offset are zero.
		binary.BigEndian.PutUint16(fctl[20:], delayNum)
		binary.BigEndian.PutUint16(fctl[22:], delayDen)
		// fctl[24]: dispose_op none; fctl[25]: blend_op source.
		if err := writeChunk(w, "fcTL", fctl); err != nil {
			return err
		}
		// The first frame is the default image and lives in IDAT; the rest
		// go to fdAT, each prefixed with its own sequence number.
		if i == 0 {
			if err := writeChunk(w, "IDAT", data); err != nil {
				return err
			}
			continue
		}
		fdat := make([]byte, 4+len(data))
		binary.BigEndian.PutUint32(fdat[0:], seq)
		seq++
		copy(fdat[4:], data)
		if err := writeChunk(w, "fdAT", fdat); err != nil {
			return err
		}
	}
	return writeChunk(w, "IEND", nil)
}

// frameData returns the zlib-compressed RGBA scanlines of one frame, each
// scanline prefixed with the "none" filter byte.
func frameData(img image.Image, width, height int) ([]byte, error) {
	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	row := make([]byte, 1+4*width)
	sb := img.Bounds()
	for y := 0; y < height; y++ {
		row[0] = 0
		for x := 0; x < width; x++ {
			c := color.NRGBAModel.Convert(img.At(sb.Min.X+x, sb.Min.Y+y)).(color.NRGBA)
			row[1+4*x] = c.R
			row[2+4*x] = c.G
			row[3+4*x] = c.B
			row[4+4*x] = c.A
		}
		if _, err := zw.Write(row); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// delayFraction converts a duration into APNG delay_num/delay_den, which are
// expressed in seconds. Milliseconds keep sub-second delays exact.
func delayFraction(d time.Duration) (num, den uint16) {
	ms := d.Milliseconds()
	if ms <= 0 {
		ms = 1000
	}
	if ms > 0xffff {
		seconds := (ms + 999) / 1000
		if seconds > 0xffff {
			seconds = 0xffff
		}
		return uint16(seconds), 1
	}
	return uint16(ms), 1000
}

// writeChunk writes one PNG chunk: length, type, data and CRC.
func writeChunk(w io.Writer, typ string, data []byte) error {
	var header [8]byte
	binary.BigEndian.PutUint32(header[0:], uint32(len(data)))
	copy(header[4:], typ)

	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	var sum [4]byte
	binary.BigEndian.PutUint32(sum[:], crc.Sum32())

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	_, err := w.Write(sum[:])
	return err
}
