// Copyright 2015 The draw2d Authors. All rights reserved.
// created: 26/06/2015 by Stani Michiels
// TODO: dashed line

package draw2dpdf

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"strconv"

	"github.com/golang/freetype/truetype"

	"github.com/jung-kurt/gofpdf"
	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
	"github.com/llgcode/draw2d/draw2dkit"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	// DPI of a pdf document is fixed at 72.
	DPI  = 72
	c255 = 255.0 / 65535.0
)

var (
	defaultFontData = draw2d.FontData{"luxi", draw2d.FontFamilySans, draw2d.FontStyleNormal}
)

var (
	caps = map[draw2d.LineCap]string{
		draw2d.RoundCap:  "round",
		draw2d.ButtCap:   "butt",
		draw2d.SquareCap: "square"}
	joins = map[draw2d.LineJoin]string{
		draw2d.RoundJoin: "round",
		draw2d.BevelJoin: "bevel",
		draw2d.MiterJoin: "miter",
	}
	imageCount uint32
	white      color.Color = color.RGBA{255, 255, 255, 255}
)

// NewPdf creates a new pdf document with the draw2d fontfolder, adds
// a page and set fill color to white.
func NewPdf(orientationStr, unitStr, sizeStr string) *gofpdf.Fpdf {
	pdf := gofpdf.New(orientationStr, unitStr, sizeStr, draw2d.GetFontFolder())
	// to be compatible with draw2d
	pdf.SetMargins(0, 0, 0)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	pdf.SetLineCapStyle("round")
	pdf.SetLineJoinStyle("round")
	pdf.SetLineWidth(1)
	pdf.AddPage()
	return pdf
}

// rgb converts a color (used by draw2d) into 3 int (used by gofpdf)
func rgb(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(float64(r) * c255), int(float64(g) * c255), int(float64(b) * c255)
}

// clearRect draws a white rectangle
func clearRect(gc *GraphicContext, x1, y1, x2, y2 float64) {
	// save state
	f := gc.Current.FillColor
	x, y := gc.pdf.GetXY()
	// cover page with white rectangle
	gc.SetFillColor(white)
	draw2dkit.Rectangle(gc, x1, y1, x2, y2)
	gc.Fill()
	// restore state
	gc.SetFillColor(f)
	gc.pdf.MoveTo(x, y)
}

// GraphicContext implements the draw2d.GraphicContext interface
// It provides draw2d with a pdf backend (based on gofpdf)
type GraphicContext struct {
	*draw2dbase.StackGraphicContext
	pdf      *gofpdf.Fpdf
	glyphBuf *truetype.GlyphBuf
	DPI      int
}

// NewGraphicContext creates a new pdf GraphicContext
func NewGraphicContext(pdf *gofpdf.Fpdf) *GraphicContext {
	gc := &GraphicContext{draw2dbase.NewStackGraphicContext(), pdf, &truetype.GlyphBuf{}, DPI}
	gc.SetDPI(DPI)
	return gc
}

// DrawImage draws an image as PNG
// TODO: add type (tp) as parameter to argument list?
func (gc *GraphicContext) DrawImage(image image.Image) {
	name := strconv.Itoa(int(imageCount))
	imageCount++
	tp := "PNG" // "JPG", "JPEG", "PNG" and "GIF"
	b := &bytes.Buffer{}
	png.Encode(b, image)
	gc.pdf.RegisterImageReader(name, tp, b)
	bounds := image.Bounds()
	x0, y0 := float64(bounds.Min.X), float64(bounds.Min.Y)
	w, h := float64(bounds.Dx()), float64(bounds.Dy())
	gc.pdf.Image(name, x0, y0, w, h, false, tp, 0, "")
}

// Clear draws a white rectangle over the whole page
func (gc *GraphicContext) Clear() {
	width, height := gc.pdf.GetPageSize()
	clearRect(gc, 0, 0, width, height)
}

// ClearRect draws a white rectangle over the specified area.
// Samples: line.
func (gc *GraphicContext) ClearRect(x1, y1, x2, y2 int) {
	clearRect(gc, float64(x1), float64(y1), float64(x2), float64(y2))
}

// recalc recalculates scale and bounds values from the font size, screen
// resolution and font metrics, and invalidates the glyph cache.
func (gc *GraphicContext) recalc() {
	// TODO: resolve properly the font size for pdf and bitmap
	gc.Current.Scale = gc.Current.FontSize * float64(gc.DPI) * (64.0 / 72.0) / 3.0
}

// SetDPI sets the DPI which influences the font size.
func (gc *GraphicContext) SetDPI(dpi int) {
	gc.DPI = dpi
	gc.recalc()
}

// GetDPI returns the DPI which influences the font size.
// (Note that gofpdf uses a fixed dpi of 72:
// https://godoc.org/code.google.com/p/gofpdf#Fpdf.PointConvert)
func (gc *GraphicContext) GetDPI() int {
	return gc.DPI
}

// GetStringBounds returns the approximate pixel bounds of the string s at x, y.
// The left edge of the em square of the first character of s
// and the baseline intersect at 0, 0 in the returned coordinates.
// Therefore the top and left coordinates may well be negative.
func (gc *GraphicContext) GetStringBounds(s string) (left, top, right, bottom float64) {
	_, h := gc.pdf.GetFontSize()
	d := gc.pdf.GetFontDesc("", "")
	if d.Ascent == 0 {
		// not defined (standard font?), use average of 81%
		top = 0.81 * h
	} else {
		top = -float64(d.Ascent) * h / float64(d.Ascent-d.Descent)
	}
	return 0, top, gc.pdf.GetStringWidth(s), top + h
}

func fUnitsToFloat64(x fixed.Int26_6) float64 {
	scaled := x << 2
	return float64(scaled/256) + float64(scaled%256)/256.0
}

// p is a truetype.Point measured in FUnits and positive Y going upwards.
// The returned value is the same thing measured in floating point and positive Y
// going downwards.
func pointToF64Point(p truetype.Point) (x, y float64) {
	return fUnitsToFloat64(p.X), -fUnitsToFloat64(p.Y)
}

// drawContour draws the given closed contour at the given sub-pixel offset.
func (gc *GraphicContext) drawContour(ps []truetype.Point, dx, dy float64) {
	if len(ps) == 0 {
		return
	}
	startX, startY := pointToF64Point(ps[0])
	gc.MoveTo(startX+dx, startY+dy)
	q0X, q0Y, on0 := startX, startY, true
	for _, p := range ps[1:] {
		qX, qY := pointToF64Point(p)
		on := p.Flags&0x01 != 0
		if on {
			if on0 {
				gc.LineTo(qX+dx, qY+dy)
			} else {
				gc.QuadCurveTo(q0X+dx, q0Y+dy, qX+dx, qY+dy)
			}
		} else {
			if on0 {
				// No-op.
			} else {
				midX := (q0X + qX) / 2
				midY := (q0Y + qY) / 2
				gc.QuadCurveTo(q0X+dx, q0Y+dy, midX+dx, midY+dy)
			}
		}
		q0X, q0Y, on0 = qX, qY, on
	}
	// Close the curve.
	if on0 {
		gc.LineTo(startX+dx, startY+dy)
	} else {
		gc.QuadCurveTo(q0X+dx, q0Y+dy, startX+dx, startY+dy)
	}
}

func (gc *GraphicContext) drawGlyph(glyph truetype.Index, dx, dy float64) error {
	if err := gc.glyphBuf.Load(gc.Current.Font, fixed.Int26_6(gc.Current.Scale), glyph, font.HintingNone); err != nil {
		return err
	}
	e0 := 0
	for _, e1 := range gc.glyphBuf.Ends {
		gc.drawContour(gc.glyphBuf.Points[e0:e1], dx, dy)
		e0 = e1
	}
	return nil
}

// CreateStringPath creates a path from the string s at x, y, and returns the string width.
func (gc *GraphicContext) CreateStringPath(s string, x, y float64) (cursor float64) {
	font, err := gc.loadCurrentFont()
	if err != nil {
		log.Println(err)
		return 0.0
	}
	startx := x
	prev, hasPrev := truetype.Index(0), false
	for _, rune := range s {
		index := font.Index(rune)
		if hasPrev {
			x += fUnitsToFloat64(font.Kern(fixed.Int26_6(gc.Current.Scale), prev, index))
		}
		err := gc.drawGlyph(index, x, y)
		if err != nil {
			log.Println(err)
			return startx - x
		}
		x += fUnitsToFloat64(font.HMetric(fixed.Int26_6(gc.Current.Scale), index).AdvanceWidth)
		prev, hasPrev = index, true
	}
	return x - startx
}

// FillString draws a string at 0, 0
func (gc *GraphicContext) FillString(text string) (cursor float64) {
	return gc.FillStringAt(text, 0, 0)
}

// FillStringAt draws a string at x, y
func (gc *GraphicContext) FillStringAt(text string, x, y float64) (cursor float64) {
	width := gc.CreateStringPath(text, x, y)
	gc.Fill()
	return width
}

// StrokeString draws a string at 0, 0 (stroking is unsupported,
// string will be filled)
func (gc *GraphicContext) StrokeString(text string) (cursor float64) {
	return gc.StrokeStringAt(text, 0, 0)
}

// StrokeStringAt draws a string at x, y (stroking is unsupported,
// string will be filled)
func (gc *GraphicContext) StrokeStringAt(text string, x, y float64) (cursor float64) {
	return gc.CreateStringPath(text, x, y)
}

func (gc *GraphicContext) loadCurrentFont() (*truetype.Font, error) {
	return gc.Current.Font, nil
}

// Stroke strokes the paths with the color specified by SetStrokeColor
func (gc *GraphicContext) Stroke(paths ...*draw2d.Path) {
	_, _, _, alphaS := gc.Current.StrokeColor.RGBA()
	gc.draw("D", alphaS, paths...)
	gc.Current.Path.Clear()
}

// Fill fills the paths with the color specified by SetFillColor
func (gc *GraphicContext) Fill(paths ...*draw2d.Path) {
	style := "F"
	if gc.Current.FillRule != draw2d.FillRuleWinding {
		style += "*"
	}
	_, _, _, alphaF := gc.Current.FillColor.RGBA()
	gc.draw(style, alphaF, paths...)
	gc.Current.Path.Clear()
}

// FillStroke first fills the paths and than strokes them
func (gc *GraphicContext) FillStroke(paths ...*draw2d.Path) {
	var rule string
	if gc.Current.FillRule != draw2d.FillRuleWinding {
		rule = "*"
	}
	_, _, _, alphaS := gc.Current.StrokeColor.RGBA()
	_, _, _, alphaF := gc.Current.FillColor.RGBA()
	if alphaS == alphaF {
		gc.draw("FD"+rule, alphaF, paths...)
	} else {
		gc.draw("F"+rule, alphaF, paths...)
		gc.draw("S", alphaS, paths...)
	}
	gc.Current.Path.Clear()
}

var logger = log.New(os.Stdout, "", log.Lshortfile)

const alphaMax = float64(0xFFFF)

// draw fills and/or strokes paths
func (gc *GraphicContext) draw(style string, alpha uint32, paths ...*draw2d.Path) {
	paths = append(paths, gc.Current.Path)
	for _, p := range paths {
		ConvertPath(p, gc.pdf)
	}
	a := float64(alpha) / alphaMax
	current, blendMode := gc.pdf.GetAlpha()
	if a != current {
		gc.pdf.SetAlpha(a, blendMode)
	}
	gc.pdf.DrawPath(style)
}

// overwrite StackGraphicContext methods

// SetStrokeColor sets the stroke color
func (gc *GraphicContext) SetStrokeColor(c color.Color) {
	gc.StackGraphicContext.SetStrokeColor(c)
	gc.pdf.SetDrawColor(rgb(c))
}

// SetFillColor sets the fill and text color
func (gc *GraphicContext) SetFillColor(c color.Color) {
	gc.StackGraphicContext.SetFillColor(c)
	gc.pdf.SetFillColor(rgb(c))
	gc.pdf.SetTextColor(rgb(c))
}

// SetFont is unsupported by the pdf graphic context, use SetFontData
// instead.
func (gc *GraphicContext) SetFont(font *truetype.Font) {
	// TODO: what to do with this api conflict between draw2d and gofpdf?!
	gc.Current.Font = font
}

// SetFontData sets the current font used to draw text. Always use
// this method, as SetFont is unsupported by the pdf graphic context.
// It is mandatory to call this method at least once before printing
// text or the resulting document will not be valid.
// It is necessary to generate a font definition file first with the
// makefont utility. It is not necessary to call this function for the
// core PDF fonts (courier, helvetica, times, zapfdingbats).
// go get github.com/jung-kurt/gofpdf/makefont
// http://godoc.org/github.com/jung-kurt/gofpdf#Fpdf.AddFont
func (gc *GraphicContext) SetFontData(fontData draw2d.FontData) {
	// TODO: call Makefont embed if json file does not exist yet
	gc.StackGraphicContext.SetFontData(fontData)
	var style string
	if fontData.Style&draw2d.FontStyleBold != 0 {
		style += "B"
	}
	if fontData.Style&draw2d.FontStyleItalic != 0 {
		style += "I"
	}
	fn := draw2d.FontFileName(fontData)
	fn = fn[:len(fn)-4]
	size, _ := gc.pdf.GetFontSize()
	gc.pdf.AddFont(fontData.Name, style, fn+".json")
	gc.pdf.SetFont(fontData.Name, style, size)
}

// SetFontSize sets the font size in points (as in ``a 12 point font'').
// TODO: resolve this with ImgGraphicContext (now done with gc.Current.Scale)
func (gc *GraphicContext) SetFontSize(fontSize float64) {
	gc.Current.FontSize = fontSize
	gc.recalc()
}

// SetLineDash sets the line dash pattern
func (gc *GraphicContext) SetLineDash(Dash []float64, DashOffset float64) {
	gc.StackGraphicContext.SetLineDash(Dash, DashOffset)
	gc.pdf.SetDashPattern(Dash, DashOffset)
}

// SetLineWidth sets the line width
func (gc *GraphicContext) SetLineWidth(LineWidth float64) {
	gc.StackGraphicContext.SetLineWidth(LineWidth)
	gc.pdf.SetLineWidth(LineWidth)
}

// SetLineCap sets the line cap (round, but or square)
func (gc *GraphicContext) SetLineCap(Cap draw2d.LineCap) {
	gc.StackGraphicContext.SetLineCap(Cap)
	gc.pdf.SetLineCapStyle(caps[Cap])
}

// SetLineJoin sets the line cap (round, bevel or miter)
func (gc *GraphicContext) SetLineJoin(Join draw2d.LineJoin) {
	gc.StackGraphicContext.SetLineJoin(Join)
	gc.pdf.SetLineJoinStyle(joins[Join])
}

// Transformations

// Scale generally scales the following text, drawings and images.
// sx and sy are the scaling factors for width and height.
// This must be placed between gc.Save() and gc.Restore(), otherwise
// the pdf is invalid.
func (gc *GraphicContext) Scale(sx, sy float64) {
	gc.StackGraphicContext.Scale(sx, sy)
	gc.pdf.TransformScale(sx*100, sy*100, 0, 0)
}

// Rotate rotates the following text, drawings and images.
// Angle is specified in radians and measured clockwise from the
// 3 o'clock position.
// This must be placed between gc.Save() and gc.Restore(), otherwise
// the pdf is invalid.
func (gc *GraphicContext) Rotate(angle float64) {
	gc.StackGraphicContext.Rotate(angle)
	gc.pdf.TransformRotate(-angle*180/math.Pi, 0, 0)
}

// Translate moves the following text, drawings and images
// horizontally and vertically by the amounts specified by tx and ty.
// This must be placed between gc.Save() and gc.Restore(), otherwise
// the pdf is invalid.
func (gc *GraphicContext) Translate(tx, ty float64) {
	gc.StackGraphicContext.Translate(tx, ty)
	gc.pdf.TransformTranslate(tx, ty)
}

// Save saves the current context stack
// (transformation, font, color,...).
func (gc *GraphicContext) Save() {
	gc.StackGraphicContext.Save()
	gc.pdf.TransformBegin()
}

// Restore restores the current context stack
// (transformation, color,...). Restoring the font is not supported.
func (gc *GraphicContext) Restore() {
	gc.pdf.TransformEnd()
	gc.StackGraphicContext.Restore()
	c := gc.Current
	gc.SetFontSize(c.FontSize)
	// gc.SetFontData(c.FontData) unsupported, causes bug (do not enable)
	gc.SetLineWidth(c.LineWidth)
	gc.SetStrokeColor(c.StrokeColor)
	gc.SetFillColor(c.FillColor)
	gc.SetFillRule(c.FillRule)
	// gc.SetLineDash(c.Dash, c.DashOffset) // TODO
	gc.SetLineCap(c.Cap)
	gc.SetLineJoin(c.Join)
	// c.Path unsupported
	// c.Font unsupported
}
