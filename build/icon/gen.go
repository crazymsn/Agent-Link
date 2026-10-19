//go:build ignore

// Icon generator for Agent Link's menu bar assets and macOS application icon.
package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

var (
	//go:embed agentlink.svg
	fullSVG string
	//go:embed agentlink-small.svg
	smallSVG string
	//go:embed deepcloud-source.jpg
	deepCloudJPEG []byte
)

func main() {
	var img image.Image
	switch {
	case len(os.Args) == 3 && os.Args[1] == "tray":
		img = tray()
	case len(os.Args) == 3 && os.Args[1] == "tray-flap":
		if err := trayFlap(os.Args[2], 44); err != nil {
			panic(err)
		}
		return
	case len(os.Args) == 4 && os.Args[1] == "tray-flap":
		n, _ := strconv.Atoi(os.Args[3])
		if err := trayFlap(os.Args[2], n); err != nil {
			panic(err)
		}
		return
	case len(os.Args) == 4 && os.Args[1] == "glyph":
		n, _ := strconv.Atoi(os.Args[2])
		img = glyph(n)
	case len(os.Args) == 4 && (os.Args[1] == "app" || os.Args[1] == "app-dark"):
		n, _ := strconv.Atoi(os.Args[2])
		img = app(n, os.Args[1] == "app-dark")
	default:
		fmt.Fprintln(os.Stderr, "usage: gen.go tray <out.png> | gen.go app|app-dark|glyph <size> <out.png>")
		os.Exit(2)
	}
	f, err := os.Create(os.Args[len(os.Args)-1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}

const trayK = 0.8

func tray() image.Image {
	const s = 44
	off := float64(s) / 2 * (1 - trayK)
	cov := coverage(parse(smallSVG), s, 8, trayK, off, off)
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	for i, a := range cov {
		img.SetNRGBA(i%s, i/s, color.NRGBA{A: uint8(a*255 + .5)})
	}
	return img
}

const (
	flapSecs, flapEvery = .9, .03
	clipBody            = "M16 0L44 0L44 44L0 44L0 26.4L10.6 26.4L13 24.2L16 19Z M29.8 18.3L26 19L22 21L17 23.5L14.6 24.7L10.6 26.4L6 29V32L9 31.5L12.5 32L16.4 31.8L19 31.4L24 28.8L28.5 24L30.6 21.4Z"
	clipTail            = "M0 0L17 0L17.2 19.6L14 25L10.6 26.4L0 26.4Z"
	clipWing            = "M29.8 18.3L26 19L22 21L17 23.5L14.6 24.7L10.6 26.4L6 29V32L9 31.5L12.5 32L16.4 31.8L19 31.4L24 28.8L28.5 24L30.6 21.4Z"
)

type key struct{ at, deg float64 }

var (
	wingKeys              = []key{{0, 0}, {.12, 10}, {.26, -3}, {.40, 8}, {.56, -2}, {.72, 3}, {1, 0}}
	tailKeys              = []key{{0, 0}, {.18, 16}, {.36, -7}, {.54, 9}, {.72, -3}, {.86, 2}, {1, 0}}
	bobKeys               = []key{{0, 0}, {.18, -4}, {.40, 2}, {.62, -1}, {1, 0}}
	wingAt, tailAt, bobAt = [2]float64{30, 19.8}, [2]float64{14, 22}, [2]float64{26, 38}
)

func trayFlap(dir string, s int) error {
	bird := parse(smallSVG)
	body, tail, wing := parse(`d="`+clipBody+`"`), parse(`d="`+clipTail+`"`), parse(`d="`+clipWing+`"`)
	n := int(math.Round(flapSecs / flapEvery))
	const ss = 8
	k := float64(s) / 44
	for f := 1; f < n; f++ {
		t := float64(f) / float64(n)
		bob, tl, wg := keyed(bobKeys, t), keyed(tailKeys, t), keyed(wingKeys, t)
		m := func(rings [][][2]float64, at [2]float64, deg float64) []bool {
			moved := make([][][2]float64, len(rings))
			for i, r := range rings {
				moved[i] = make([][2]float64, len(r))
				for j, p := range r {
					q := turn(turn(p, at, deg), bobAt, bob)
					moved[i][j] = [2]float64{22 + (q[0]-22)*trayK, 22 + (q[1]-22)*trayK}
				}
			}
			return mask(moved, s*ss, k*ss)
		}
		pieces := [][2][]bool{{m(bird, tailAt, 0), m(body, tailAt, 0)}, {m(bird, tailAt, tl), m(tail, tailAt, tl)}, {m(bird, wingAt, wg), m(wing, wingAt, wg)}}
		nn := s * ss
		img := image.NewNRGBA(image.Rect(0, 0, s, s))
		for py := 0; py < s; py++ {
			for px := 0; px < s; px++ {
				hit := 0
				for sy := 0; sy < ss; sy++ {
					for sx := 0; sx < ss; sx++ {
						i := (py*ss+sy)*nn + px*ss + sx
						for _, pc := range pieces {
							if pc[0][i] && pc[1][i] {
								hit++
								break
							}
						}
					}
				}
				img.SetNRGBA(px, py, color.NRGBA{A: uint8(float64(hit)/(ss*ss)*255 + .5)})
			}
		}
		out, err := os.Create(fmt.Sprintf("%s/flap-%02d.png", dir, f))
		if err != nil {
			return err
		}
		err = png.Encode(out, img)
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func keyed(ks []key, t float64) float64 {
	for i := 1; i < len(ks); i++ {
		if t <= ks[i].at {
			a, b := ks[i-1], ks[i]
			return a.deg + (b.deg-a.deg)*ease((t-a.at)/(b.at-a.at))
		}
	}
	return ks[len(ks)-1].deg
}
func ease(x float64) float64 {
	bez := func(u, p1, p2 float64) float64 { return 3*u*(1-u)*(1-u)*p1 + 3*u*u*(1-u)*p2 + u*u*u }
	lo, hi := 0., 1.
	for range 40 {
		m := (lo + hi) / 2
		if bez(m, .3, .3) < x {
			lo = m
		} else {
			hi = m
		}
	}
	return bez((lo+hi)/2, 0, 1)
}
func turn(p, o [2]float64, deg float64) [2]float64 {
	a := deg * math.Pi / 180
	s, c := math.Sin(a), math.Cos(a)
	dx, dy := p[0]-o[0], p[1]-o[1]
	return [2]float64{o[0] + dx*c - dy*s, o[1] + dx*s + dy*c}
}

func mask(rings [][][2]float64, n int, k float64) []bool {
	out := make([]bool, n*n)
	var xs []float64
	for y := 0; y < n; y++ {
		v := (float64(y) + .5) / k
		xs = xs[:0]
		for _, r := range rings {
			for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
				a, b := r[j], r[i]
				if (a[1] > v) != (b[1] > v) {
					xs = append(xs, (a[0]+(v-a[1])*(b[0]-a[0])/(b[1]-a[1]))*k)
				}
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			lo := int(math.Max(0, math.Ceil(xs[i]-.5)))
			hi := int(math.Min(float64(n), math.Ceil(xs[i+1]-.5)))
			for x := lo; x < hi; x++ {
				out[y*n+x] = true
			}
		}
	}
	return out
}

func glyph(n int) image.Image { return deepCloudMark(n) }

func app(n int, dark bool) image.Image {
	size := n * 4
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px, py := (float64(x)+.5)/float64(size), (float64(y)+.5)/float64(size)
			d := iconDistance(px, py)
			shadow := .14 * math.Exp(-.5*math.Pow(math.Max(0, iconDistance(px, py-.012))/.014, 2))
			pixel := color.NRGBA{R: 16, G: 22, B: 32, A: uint8(shadow * 255)}
			if d <= 0 {
				t := math.Max(0, math.Min(1, (py-.10)/.80))
				edge := math.Exp(d / .0014)
				if dark {
					v := 58 - 25*t + 15*edge
					pixel = color.NRGBA{R: uint8(v), G: uint8(v + 2), B: uint8(v + 5), A: 255}
				} else {
					v := 254 - 15*t + 4*math.Exp(-math.Pow((px-.30)/.65, 2)-math.Pow((py-.18)/.55, 2))
					v = math.Max(0, math.Min(255, v+5*edge))
					pixel = color.NRGBA{R: uint8(v), G: uint8(v), B: uint8(math.Min(255, v+1)), A: 255}
				}
			}
			canvas.SetNRGBA(x, y, pixel)
		}
	}
	mark := deepCloudMark(size)
	at := image.Pt((size-mark.Bounds().Dx())/2, (size-mark.Bounds().Dy())/2)
	draw.Draw(canvas, image.Rectangle{Min: at, Max: at.Add(mark.Bounds().Size())}, mark, image.Point{}, draw.Over)
	return reduceIcon(canvas, n)
}

func iconDistance(x, y float64) float64 {
	x, y = math.Abs(x-.5), math.Abs(y-.5)
	const half, radius, power = .40, .19, 2.6
	qx, qy := math.Max(x-(half-radius), 0), math.Max(y-(half-radius), 0)
	if qx == 0 && qy == 0 {
		return math.Max(x, y) - half
	}
	return math.Pow(math.Pow(qx, power)+math.Pow(qy, power), 1/power) - radius
}

func deepCloudLogo() *image.NRGBA {
	src, err := jpeg.Decode(bytes.NewReader(deepCloudJPEG))
	if err != nil {
		panic(err)
	}
	bounds := src.Bounds()
	ink := image.Rectangle{Min: bounds.Max, Max: bounds.Min}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			if (r*30+g*59+b*11)/100 < 128*257 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if ink.Empty() {
		panic("DeepCloud artwork has no visible logo")
	}
	logo := image.NewNRGBA(image.Rect(0, 0, ink.Dx(), ink.Dy()))
	for y := 0; y < ink.Dy(); y++ {
		for x := 0; x < ink.Dx(); x++ {
			r, g, b, _ := src.At(ink.Min.X+x, ink.Min.Y+y).RGBA()
			luma := float64((r*30+g*59+b*11)/100) / 257
			alpha := math.Max(0, math.Min(1, (245-luma)/225))
			logo.SetNRGBA(x, y, color.NRGBA{A: uint8(alpha * 255)})
		}
	}
	return logo
}

func deepCloudMark(n int) image.Image {
	logo := deepCloudLogo()
	width := int(float64(n) * .60)
	height := int(float64(logo.Bounds().Dy()) * float64(width) / float64(logo.Bounds().Dx()))
	resized := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			resized.Set(x, y, logo.At(x*logo.Bounds().Dx()/width, y*logo.Bounds().Dy()/height))
		}
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, n, n))
	at := image.Pt((n-width)/2, (n-height)/2-int(float64(n)*.015))
	draw.Draw(canvas, image.Rectangle{Min: at, Max: at.Add(resized.Bounds().Size())}, resized, image.Point{}, draw.Over)
	return canvas
}

func reduceIcon(src *image.NRGBA, n int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a uint32
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					p := src.NRGBAAt(x*4+sx, y*4+sy)
					r += uint32(p.R) * uint32(p.A)
					g += uint32(p.G) * uint32(p.A)
					b += uint32(p.B) * uint32(p.A)
					a += uint32(p.A)
				}
			}
			if a > 0 {
				out.SetNRGBA(x, y, color.NRGBA{R: uint8(r / a), G: uint8(g / a), B: uint8(b / a), A: uint8((a + 8) / 16)})
			}
		}
	}
	return out
}

func coverage(rings [][][2]float64, n, ss int, k, ox, oy float64) []float64 {
	cov := make([]float64, n*n)
	per := 1 / float64(ss*ss)
	var xs []float64
	for y := 0; y < n*ss; y++ {
		v := ((float64(y)+.5)/float64(ss) - oy) / k
		xs = xs[:0]
		for _, r := range rings {
			for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
				a, b := r[j], r[i]
				if (a[1] > v) != (b[1] > v) {
					u := a[0] + (v-a[1])*(b[0]-a[0])/(b[1]-a[1])
					xs = append(xs, (u*k+ox)*float64(ss))
				}
			}
		}
		sort.Float64s(xs)
		row := cov[(y/ss)*n:][:n]
		for i := 0; i+1 < len(xs); i += 2 {
			lo := int(math.Max(0, math.Ceil(xs[i]-.5)))
			hi := int(math.Min(float64(n*ss), math.Ceil(xs[i+1]-.5)))
			for x := lo; x < hi; x++ {
				row[x/ss] += per
			}
		}
	}
	return cov
}

func parse(svg string) [][][2]float64 {
	d := svg[strings.Index(svg, ` d="`)+4:]
	d = d[:strings.IndexByte(d, '"')]
	f := strings.Fields(strings.NewReplacer("M", " M ", "L", " L ", "C", " C ", "Z", " Z ").Replace(d))
	num := func(i int) float64 { x, _ := strconv.ParseFloat(f[i], 64); return x }
	var rings [][][2]float64
	var pts [][2]float64
	cmd := ""
	for i := 0; i < len(f); {
		if c := f[i]; c == "M" || c == "L" || c == "C" || c == "Z" {
			cmd = c
			i++
		}
		switch cmd {
		case "M", "L":
			if cmd == "M" && len(pts) > 0 {
				rings, pts = append(rings, pts), nil
			}
			pts = append(pts, [2]float64{num(i), num(i + 1)})
			i += 2
			cmd = "L"
		case "C":
			p := pts[len(pts)-1]
			c := [8]float64{p[0], p[1], num(i), num(i + 1), num(i + 2), num(i + 3), num(i + 4), num(i + 5)}
			for s := 1; s <= 8; s++ {
				t := float64(s) / 8
				m := 1 - t
				pts = append(pts, [2]float64{m*m*m*c[0] + 3*m*m*t*c[2] + 3*m*t*t*c[4] + t*t*t*c[6], m*m*m*c[1] + 3*m*m*t*c[3] + 3*m*t*t*c[5] + t*t*t*c[7]})
			}
			i += 6
		case "Z":
			if len(pts) > 0 {
				rings, pts = append(rings, pts), nil
			}
		}
	}
	if len(pts) > 0 {
		rings = append(rings, pts)
	}
	return rings
}

func roundRect(px, py, x, y, w, h, r float64) bool {
	if px < x || py < y || px > x+w || py > y+h {
		return false
	}
	cx := math.Max(x+r, math.Min(px, x+w-r))
	cy := math.Max(y+r, math.Min(py, y+h-r))
	return math.Hypot(px-cx, py-cy) <= r
}

func mustDeepCloud() image.Image {
	f, err := jpeg.Decode(bytes.NewReader(deepCloudJPEG))
	if err != nil {
		panic(err)
	}
	return f
}
