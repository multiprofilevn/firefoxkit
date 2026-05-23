package input

import (
	"math"
	"math/rand"
)

// Point is one step on a cursor path.
type Point struct{ X, Y float64 }

func stepCount(x1, y1, x2, y2 float64) int {
	dist := math.Hypot(x2-x1, y2-y1)
	steps := int(dist / 25)
	if steps < 3 {
		steps = 3
	}
	if steps > 25 {
		steps = 25
	}
	return steps
}

func interpolate(x1, y1, x2, y2 float64, steps int) []Point {
	if steps < 1 {
		steps = 1
	}
	out := make([]Point, 0, steps)
	jitter := 2.0
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		px := x1 + (x2-x1)*t
		py := y1 + (y2-y1)*t
		if i < steps {
			px += (rand.Float64()*2 - 1) * jitter
			py += (rand.Float64()*2 - 1) * jitter
		}
		out = append(out, Point{X: px, Y: py})
	}
	return out
}

func humanStepCount(x1, y1, x2, y2 float64) int {
	dist := math.Hypot(x2-x1, y2-y1)
	steps := int(dist / 12)
	if steps < 8 {
		return 8
	}
	if steps > 80 {
		return 80
	}
	return steps
}

func easeInOutCubic(t float64) float64 { return 3*t*t - 2*t*t*t }

func randomControlPoints(x1, y1, x2, y2 float64) (Point, Point) {
	dx := x2 - x1
	dy := y2 - y1
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return Point{X: x1, Y: y1}, Point{X: x2, Y: y2}
	}
	nx := -dy / dist
	ny := dx / dist
	mag1 := dist * (0.15 + rand.Float64()*0.10)
	mag2 := dist * (0.15 + rand.Float64()*0.10)
	if rand.Intn(2) == 0 {
		mag1 = -mag1
	}
	if rand.Intn(2) == 0 {
		mag2 = -mag2
	}
	cp1 := Point{X: x1 + dx/3 + nx*mag1, Y: y1 + dy/3 + ny*mag1}
	cp2 := Point{X: x1 + 2*dx/3 + nx*mag2, Y: y1 + 2*dy/3 + ny*mag2}
	return cp1, cp2
}

func bezierPath(x1, y1, x2, y2 float64, steps int) []Point {
	if steps < 1 {
		steps = 1
	}
	cp1, cp2 := randomControlPoints(x1, y1, x2, y2)
	out := make([]Point, 0, steps)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		if i == steps {
			out = append(out, Point{X: x2, Y: y2})
			break
		}
		te := easeInOutCubic(t)
		out = append(out, cubicBezier(te, x1, y1, cp1.X, cp1.Y, cp2.X, cp2.Y, x2, y2))
	}
	return out
}

func cubicBezier(t, p0x, p0y, p1x, p1y, p2x, p2y, p3x, p3y float64) Point {
	u := 1 - t
	uu := u * u
	uuu := uu * u
	tt := t * t
	ttt := tt * t
	x := uuu*p0x + 3*uu*t*p1x + 3*u*tt*p2x + ttt*p3x
	y := uuu*p0y + 3*uu*t*p1y + 3*u*tt*p2y + ttt*p3y
	return Point{X: x, Y: y}
}
