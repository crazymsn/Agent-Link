package gateway

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"

	"github.com/crazymsn/Agent-Link/internal/provider"
)

// Codex's own client sizes every inline image it sends to ChatGPT before
// sending it (openai/codex, codex-rs/utils/image and
// core/src/image_preparation.rs): one larger than its limits is scaled
// down with a triangle filter and written again in its own format — PNG
// as PNG, JPEG as JPEG at quality 85 — and a GIF as PNG; one that fits
// goes as it came. Another agent signed in to the same ChatGPT account
// through agentlink sends its screenshots whole, and ChatGPT dropped
// ylorn's (Discord, v0.1.1179) once one took a turn past about 1 MiB.
// codexImagesFit does what Codex's client does, for a Codex account only.
//
// The limits are Codex's: detail auto, high or none fits 2048 px a side
// and 2500 patches of 32 px; "original" 6000 px and 10000 patches. Codex
// refuses "low" itself, and it is left as sent. WebP, which Go's standard
// library doesn't decode, is left as sent too.
func codexImagesFit(p provider.Provider, to provider.Protocol, body []byte) []byte {
	if p.Account == nil || p.Account.Agent != "codex" || to != provider.Responses || !bytes.Contains(body, []byte(`input_image`)) {
		return body
	}
	// One walk of the body: each input item, and the parts of the items
	// with an image in them, read where they sit. Looking each part up
	// again by its path from the top read the body from its start for
	// every image: in a Computer Use turn of 160 screenshots, gigabytes of
	// JSON read for one request (fflake33, Discord).
	var jobs []fitJob
	gjson.GetBytes(body, "input").ForEach(func(_, item gjson.Result) bool {
		if !strings.Contains(item.Raw, "input_image") {
			return true
		}
		item.ForEach(func(key, parts gjson.Result) bool {
			if key.Str != "content" && key.Str != "output" || !parts.IsArray() {
				return true
			}
			parts.ForEach(func(_, part gjson.Result) bool {
				// the part's members read once, not once for each
				var kind, detail, url gjson.Result
				part.ForEach(func(k, v gjson.Result) bool {
					// the first of a key given twice, as Get reads it
					switch {
					case k.Str == "type" && !kind.Exists():
						kind = v
					case k.Str == "detail" && !detail.Exists():
						detail = v
					case k.Str == "image_url" && !url.Exists():
						url = v
					}
					return true
				})
				if kind.String() != "input_image" {
					return true
				}
				end := url.Index + len(url.Raw)
				if url.Type != gjson.String || url.Index <= 0 || end > len(body) || string(body[url.Index:end]) != url.Raw || !strings.HasPrefix(url.Str, "data:") {
					return true
				}
				if limits, ok := codexImageLimits(detail.String()); ok {
					jobs = append(jobs, fitJob{at: url.Index, end: end, url: url.Str, l: limits})
				}
				return true
			})
			return true
		})
		return true
	})
	if len(jobs) == 0 {
		return body
	}
	fitAll(jobs)
	slices.SortFunc(jobs, func(a, b fitJob) int { return a.at - b.at })
	// the rest of the request goes byte for byte as it came, for the
	// prompt cache
	var out bytes.Buffer
	last, swapped := 0, false
	for _, j := range jobs {
		if !j.changed || j.at < last {
			continue
		}
		if !swapped {
			out.Grow(len(body))
			swapped = true
		}
		out.Write(body[last:j.at])
		out.WriteByte('"')
		out.WriteString(j.with)
		out.WriteByte('"')
		last = j.end
	}
	if !swapped {
		return body
	}
	out.Write(body[last:])
	return out.Bytes()
}

// fitJob is one image of a request: where its data URL's JSON string
// sits, the URL, the limits it is fitted to, and what it is sent as.
type fitJob struct {
	at, end int
	url     string
	l       imageLimits
	key     [32]byte
	with    string
	changed bool
}

// fitWorkers is how many images of a request are made again at once:
// each holds a decoded screenshot and its resize, near 100 MB for a
// Retina one.
const fitWorkers = 4

// fitAll fits each job's image: as it was kept (fitted), or else made
// again, the images not kept made side by side.
func fitAll(jobs []fitJob) {
	fitted.letGo()
	var todo []int
	for i := range jobs {
		j := &jobs[i]
		comma := strings.IndexByte(j.url, ',')
		if comma < 0 || !strings.HasSuffix(j.url[:comma], ";base64") {
			continue
		}
		j.key = fitKey(j.url, j.l)
		if e, ok := fitted.get(j.key); ok {
			j.with, j.changed = e.url(), e.data != nil
			continue
		}
		todo = append(todo, i)
	}
	if len(todo) == 0 {
		return
	}
	// the same screenshot twice in a request is made once
	made := map[[32]byte]*fitEntry{}
	var order []int
	for _, i := range todo {
		if _, ok := made[jobs[i].key]; !ok {
			made[jobs[i].key] = &fitEntry{}
			order = append(order, i)
		}
	}
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(fitWorkers, runtime.GOMAXPROCS(0), len(order)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				j := jobs[i]
				*made[j.key] = codexImageRefit(j.url[strings.IndexByte(j.url, ',')+1:], j.l)
			}
		}()
	}
	for _, i := range order {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, i := range order {
		fitted.put(jobs[i].key, *made[jobs[i].key])
	}
	for _, i := range todo {
		e := made[jobs[i].key]
		jobs[i].with, jobs[i].changed = e.url(), e.data != nil
	}
}

type imageLimits struct{ side, patches int }

// codexPatch is the patch a Responses model reads an image in.
const codexPatch = 32

func codexImageLimits(detail string) (imageLimits, bool) {
	switch detail {
	case "", "auto", "high":
		return imageLimits{2048, 2500}, true
	case "original":
		return imageLimits{6000, 10000}, true
	}
	return imageLimits{}, false
}

func (l imageLimits) fit(w, h int) bool {
	pw, ph := (w+codexPatch-1)/codexPatch, (h+codexPatch-1)/codexPatch
	return w <= l.side && h <= l.side && pw*ph <= l.patches
}

// size is the size Codex's prompt_image_output_dimensions_for_limits
// gives an image of w by h: as it is when it fits; scaled to the longest
// side allowed; and then, if it still has too many patches, scaled by
// area with the patch grid rounded down.
func (l imageLimits) size(w, h int) (int, int) {
	w, h = max(w, 1), max(h, 1)
	if l.fit(w, h) {
		return w, h
	}
	scale := math.Min(float64(l.side)/float64(max(w, h)), 1)
	w = max(int(math.Round(float64(w)*scale)), 1)
	h = max(int(math.Round(float64(h)*scale)), 1)
	if l.fit(w, h) {
		return w, h
	}
	fw, fh := float64(w), float64(h)
	scale = math.Sqrt(codexPatch * codexPatch * float64(l.patches) / fw / fh)
	pw, ph := fw*scale/codexPatch, fh*scale/codexPatch
	scale *= math.Min(math.Floor(pw)/pw, math.Floor(ph)/ph)
	return max(int(math.Floor(fw*scale)), 1), max(int(math.Floor(fh*scale)), 1)
}

// codexImageMaxPixels keeps a request from decoding an image too large
// to hold: one past it goes as it came.
const codexImageMaxPixels = 80 << 20

// fitEntry is what an image is sent as: data in mime, or, with no data,
// the image as it came.
type fitEntry struct {
	mime string
	data []byte
}

// url is the entry's data URL, or "" for the image as it came.
func (e fitEntry) url() string {
	if e.data == nil {
		return ""
	}
	return "data:" + e.mime + ";base64," + base64.StdEncoding.EncodeToString(e.data)
}

// fitKey names an image's data URL fitted to l.
func fitKey(url string, l imageLimits) [32]byte {
	h := sha256.New()
	var n [16]byte
	binary.LittleEndian.PutUint64(n[:8], uint64(l.side))
	binary.LittleEndian.PutUint64(n[8:], uint64(l.patches))
	h.Write(n[:])
	io.WriteString(h, url)
	var k [32]byte
	h.Sum(k[:0])
	return k
}

// fitted holds what codexImageRefit made of the images a conversation
// sends again on every turn, so that a past screenshot is decoded, resized
// and encoded once, not on each turn. It is bounded by the bytes it holds
// (fittedBudget), dropping the least recently used first, and drops what
// no request has sent for fittedIdle. The 64-entry map it replaces was
// cleared when full, so past 64 screenshots every turn made each of them
// again (fflake33, Discord).
//
// A request looks up all its images before it keeps any it made, so a
// conversation larger than the budget still finds the images kept since
// its last turn, rather than dropping each just before it is sent again.
var fitted = &fitCache{m: map[[32]byte]*list.Element{}}

// fittedBudget is how many bytes of fitted images are kept, and
// fittedIdle how long one is kept unsent. fittedMost bounds the entries,
// as most images go as they came and hold no bytes.
var (
	fittedBudget = 256 << 20
	fittedIdle   = time.Hour
	fittedMost   = 8192
)

type fitCache struct {
	sync.Mutex
	m     map[[32]byte]*list.Element
	order list.List // of *fitItem, the most recently used first
	bytes int
}

type fitItem struct {
	key  [32]byte
	e    fitEntry
	used time.Time
}

// letGo drops what has gone unsent for fittedIdle.
func (c *fitCache) letGo() {
	c.Lock()
	defer c.Unlock()
	now := time.Now()
	for back := c.order.Back(); back != nil && now.Sub(back.Value.(*fitItem).used) > fittedIdle; back = c.order.Back() {
		c.drop(back)
	}
}

func (c *fitCache) get(k [32]byte) (fitEntry, bool) {
	c.Lock()
	defer c.Unlock()
	el, ok := c.m[k]
	if !ok {
		return fitEntry{}, false
	}
	it := el.Value.(*fitItem)
	it.used = time.Now()
	c.order.MoveToFront(el)
	return it.e, true
}

func (c *fitCache) put(k [32]byte, e fitEntry) {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.m[k]; ok || len(e.data) > fittedBudget {
		return
	}
	for c.bytes+len(e.data) > fittedBudget || len(c.m) >= fittedMost {
		c.drop(c.order.Back())
	}
	c.m[k] = c.order.PushFront(&fitItem{key: k, e: e, used: time.Now()})
	c.bytes += len(e.data)
}

func (c *fitCache) drop(el *list.Element) {
	it := c.order.Remove(el).(*fitItem)
	delete(c.m, it.key)
	c.bytes -= len(it.e.data)
}

// refits counts the images codexImageRefit decodes and writes again.
var refits atomic.Int64

// codexImageRefit is an image's base64 resized and written again, or an
// entry with no data when it goes as it came.
func codexImageRefit(b64 string, l imageLimits) fitEntry {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fitEntry{}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width*cfg.Height > codexImageMaxPixels {
		return fitEntry{}
	}
	w, h := l.size(cfg.Width, cfg.Height)
	if w == cfg.Width && h == cfg.Height && format != "gif" {
		return fitEntry{}
	}
	refits.Add(1)
	var src image.Image
	switch format {
	case "png":
		src, err = png.Decode(bytes.NewReader(raw))
	case "jpeg":
		src, err = jpeg.Decode(bytes.NewReader(raw))
	case "gif":
		src, err = gif.Decode(bytes.NewReader(raw))
	default:
		return fitEntry{}
	}
	if err != nil {
		return fitEntry{}
	}
	if w != cfg.Width || h != cfg.Height {
		src = triangleResize(src, w, h)
	}
	var out bytes.Buffer
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
		err = jpeg.Encode(&out, src, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&out, src)
	}
	if err != nil {
		return fitEntry{}
	}
	return fitEntry{mime: mime, data: out.Bytes()}
}

// triangleResize scales src to w by h with a triangle (linear) filter
// widened by the scale when shrinking, as the image crate's
// FilterType::Triangle does, one axis and then the other.
func triangleResize(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	rgba, ok := src.(*image.RGBA)
	if !ok || rgba.Rect.Min != (image.Point{}) {
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Rect, src, b.Min, draw.Src)
	}
	sw, sh := b.Dx(), b.Dy()
	// across: sw×sh → w×sh
	mid := make([]float32, w*sh*4)
	for x, k := range triangleWeights(sw, w) {
		for y := 0; y < sh; y++ {
			row := rgba.Pix[y*rgba.Stride:]
			var r, g, bl, a float32
			for i, wt := range k.w {
				p := row[(k.from+i)*4:]
				r += wt * float32(p[0])
				g += wt * float32(p[1])
				bl += wt * float32(p[2])
				a += wt * float32(p[3])
			}
			o := (y*w + x) * 4
			mid[o], mid[o+1], mid[o+2], mid[o+3] = r, g, bl, a
		}
	}
	// down: w×sh → w×h
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y, k := range triangleWeights(sh, h) {
		for x := 0; x < w; x++ {
			var c [4]float32
			for i, wt := range k.w {
				o := ((k.from+i)*w + x) * 4
				c[0] += wt * mid[o]
				c[1] += wt * mid[o+1]
				c[2] += wt * mid[o+2]
				c[3] += wt * mid[o+3]
			}
			o := y*dst.Stride + x*4
			for ch := range c {
				dst.Pix[o+ch] = uint8(min(max(math.Round(float64(c[ch])), 0), 255))
			}
			// premultiplied: no channel past its alpha
			for ch := 0; ch < 3; ch++ {
				dst.Pix[o+ch] = min(dst.Pix[o+ch], dst.Pix[o+3])
			}
		}
	}
	return dst
}

type tap struct {
	from int
	w    []float32
}

// triangleWeights are, for each of n outputs along an axis of src
// pixels, the source pixels it reads from and their weights.
func triangleWeights(src, n int) []tap {
	ratio := float64(src) / float64(n)
	scale := math.Max(ratio, 1)
	support := scale // the triangle's own support is 1
	taps := make([]tap, n)
	for i := range taps {
		center := (float64(i) + 0.5) * ratio
		from := max(int(math.Floor(center-support)), 0)
		to := min(int(math.Ceil(center+support)), src)
		var ws []float32
		var sum float64
		for j := from; j < to; j++ {
			wt := math.Max(1-math.Abs((float64(j)+0.5-center)/scale), 0)
			ws = append(ws, float32(wt))
			sum += wt
		}
		if sum == 0 {
			j := min(max(int(center), 0), src-1)
			taps[i] = tap{j, []float32{1}}
			continue
		}
		for k := range ws {
			ws[k] = float32(float64(ws[k]) / sum)
		}
		taps[i] = tap{from, ws}
	}
	return taps
}
