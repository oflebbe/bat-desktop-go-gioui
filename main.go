package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"math/cmplx"
	"os"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"github.com/argusdusty/gofft"
)

func hue2rgb(p float64, q float64, t float64) float64 {
	if t < 0 {
		t += 1.0
	}
	if t > 1 {
		t -= 1.0
	}
	if t < (1.0 / 6) {
		return p + (q-p)*6*t
	}
	if t < (1. / 2.) {
		return q
	}
	if t < (2. / 3.) {
		return p + (q-p)*((2./3.)-t)*6
	}
	return p
}

func hsl_to_color(h float64, s float64, l float64) color.RGBA {
	var r float64
	var g float64
	var b float64

	if s == 0 {
		r = l
		g = l
		b = l // achromatic
	} else {

		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}

		p := 2*l - q

		r = hue2rgb(p, q, h+1/3.)
		g = hue2rgb(p, q, h)
		b = hue2rgb(p, q, h-1/3.)
	}

	return color.RGBA{R: (uint8)(r * 255), G: (uint8)(g * 255), B: (uint8)(b * 255), A: 255}
}

func create_image(filename string) image.Image {

	in, err := os.Open(filename)
	if err != nil {
		log.Fatalf("open %s: %v", os.Args[1], err)
	}
	defer in.Close()

	fin, err := in.Stat()
	if err != nil {
		log.Fatalf("stat: %v", err)
	}

	size := fin.Size()
	if size%2 != 0 {
		log.Fatalf("invalid file size %d\n", size)
	}

	len := (int)(size / 2)

	buf := make([]uint16, len)
	err = binary.Read(in, binary.LittleEndian, buf)
	if err != nil {
		log.Fatalf("Read %v", err)
	}

	SIZE := 512
	a0 := 25. / 46.
	R := 128

	window := make([]float64, SIZE)
	for i := 0; i < SIZE; i++ {
		window[i] = a0 - (1.0-a0)*(math.Cos(2.0*math.Pi*(float64)(i)/(float64)(SIZE-1)))
	}

	work := make([]complex128, SIZE)

	width := (int)(len) / R
	height := SIZE / 2
	image := image.NewNRGBA(image.Rect(0, 0, width, height))
	min := 100.0
	max := 0.0

	// start 15kHz
	// 0 - 125 kHz -> 512 Px
	// 15 / 125 * 512
	for i := 0; i < width; i++ {
		index := ((len - SIZE) * i) / (width - 1)
		for j := 0; j < SIZE; j++ {
			work[j] = complex(((float64)(buf[index+j])-2048.0)*window[j], 0)
		}
		gofft.FFT(work)

		count_more_05 := 0
		for j := 0; j < height; j++ {
			z := math.Log10(cmplx.Abs(work[j]))

			ang := (z + 0.4) / (6 + 0.4)

			c := hsl_to_color(ang, 1., ang)

			if z > max {
				max = z
			}
			if z < min {
				min = z
			}
			if ang > 0.5 {
				count_more_05++
			}
			image.Set(i, height-j-1, c)
		}

		/* if (count_more_05 > height*0.5) {

		   std::string file_str = std::format("more75%d.txt", i);
		   std::ofstream outfile( file_str);

		   for (int j = 0; j < SIZE; j++) {
		     outfile <<  (buf[index + j ]-2048) * window[j] << "\n";
		   }
		 } */
	}
	fmt.Printf("%g %g\n", min, max)
	return image
}

func main() {
	go func() {
		window := new(app.Window)
		err := run(window)
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

var selected image.Rectangle
var selecting bool

// viewport models a region of a larger space. Offset is the location
// of the upper-left corner of the view within the larger space. size
// is the dimensions of the viewport within the larger space.
type viewport struct {
	offset f32.Point
	size   f32.Point
}

var view *viewport

// subview modifies v to describe a smaller region by zooming into the
// space described by v using other.
func (v *viewport) subview(other *viewport) {
	v.offset.X += other.offset.X * v.size.X
	v.offset.Y += other.offset.Y * v.size.Y
	v.size.X *= other.size.X
	v.size.Y *= other.size.Y
}

func layoutSelectionLayer(gtx layout.Context) layout.Dimensions {
	for {
		event, ok := gtx.Event(pointer.Filter{
			Target: &selected,
			Kinds:  pointer.Press | pointer.Release | pointer.Drag,
		})
		if !ok {
			break
		}
		switch event := event.(type) {
		case pointer.Event:
			var intPt image.Point
			intPt.X = int(event.Position.X)
			intPt.Y = int(event.Position.Y)
			switch event.Kind {
			case pointer.Press:
				selecting = true
				selected.Min = intPt
				selected.Max = intPt
			case pointer.Release:
				selecting = false
				newView := &viewport{
					offset: f32.Point{
						X: float32(selected.Min.X) / float32(gtx.Constraints.Max.X),
						Y: float32(selected.Min.Y) / float32(gtx.Constraints.Max.Y),
					},
					size: f32.Point{
						X: float32(selected.Dx()) / float32(gtx.Constraints.Max.X),
						Y: float32(selected.Dy()) / float32(gtx.Constraints.Max.Y),
					},
				}
				if view == nil {
					view = newView
				} else {
					view.subview(newView)
				}
			case pointer.Cancel:
				selecting = false
				selected = image.Rectangle{}
			}
		}
	}
	if selecting {
		paint.FillShape(gtx.Ops, color.NRGBA{R: 255, A: 100}, clip.Rect(selected).Op())
	}
	pr := clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops)
	pointer.CursorCrosshair.Add(gtx.Ops)
	event.Op(gtx.Ops, &selected)
	pr.Pop()

	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func run(window *app.Window) error {
	// theme := material.NewTheme()
	image := create_image(os.Args[1])

	imageOp := paint.NewImageOp(image)
	im := widget.Image{Src: imageOp}

	var ops op.Ops
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			// This graphics context is used for managing the rendering state.
			gtx := app.NewContext(&ops, e)
			layoutSelectionLayer(gtx)
			im.Layout(gtx)

			// Pass the drawing operations to the GPU.
			e.Frame(gtx.Ops)
		}
	}
}
