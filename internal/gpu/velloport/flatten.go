// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Port of vello_shaders/src/cpu/flatten.rs — Euler Spiral based curve flattening.
// Original: Copyright 2023 the Vello Authors, Apache-2.0 OR MIT OR Unlicense.
//
// This file implements fill-only flattening (offset=0). Stroke expansion
// (parallel curves, caps, joins) is not yet ported.

package velloport

import "math"

// Flatten constants matching vello_shaders/src/cpu/flatten.rs.
const (
	derivThresh float32 = 1e-6
	derivEps    float32 = 1e-6
	subdivLimit float32 = 1.0 / 65536.0
	flattenTol  float32 = 0.25
)

// CubicBezier represents a cubic Bezier curve segment.
type CubicBezier struct {
	P0, P1, P2, P3 [2]float32
}

// FlattenFill flattens a sequence of cubic Bezier curves into line segments
// using Vello's Euler Spiral based adaptive subdivision.
//
// This is a direct port of vello_shaders/src/cpu/flatten.rs flatten_euler()
// for the fill case (offset=0, identity transform).
func FlattenFill(cubics []CubicBezier) []LineSoup {
	var lines []LineSoup
	for _, c := range cubics {
		p0 := vec2{c.P0[0], c.P0[1]}
		p1 := vec2{c.P1[0], c.P1[1]}
		p2 := vec2{c.P2[0], c.P2[1]}
		p3 := vec2{c.P3[0], c.P3[1]}
		flattenEulerFill(p0, p1, p2, p3, &lines)
	}
	return lines
}

// evalCubicAndDeriv evaluates both the point and derivative of a cubic Bezier at parameter t.
func evalCubicAndDeriv(p0, p1, p2, p3 vec2, t float32) (vec2, vec2) {
	m := 1.0 - t
	mm := m * m
	mt := m * t
	tt := t * t
	// p = p0*(1-t)^3 + p1*3*(1-t)^2*t + p2*3*(1-t)*t^2 + p3*t^3
	p := p0.mul(mm * m).add(p1.mul(3 * mm).add(p2.mul(3 * mt)).add(p3.mul(tt)).mul(t))
	// q = (p1-p0)*(1-t)^2 + (p2-p1)*2*(1-t)*t + (p3-p2)*t^2
	q := p1.sub(p0).mul(mm).add(p2.sub(p1).mul(2 * mt)).add(p3.sub(p2).mul(tt))
	return p, q
}

// flattenEulerFill flattens a single cubic Bezier for fill (offset=0, identity transform).
// Direct port of flatten_euler() from flatten.rs.
func flattenEulerFill(p0, p1, p2, p3 vec2, lines *[]LineSoup) {
	// Drop zero-length lines.
	if p0 == p1 && p0 == p2 && p0 == p3 {
		return
	}

	var t0u uint32
	dt := float32(1.0)
	lastP := p0
	lastQ := p1.sub(p0)

	if lastQ.lengthSq() < derivThresh*derivThresh {
		_, lastQ = evalCubicAndDeriv(p0, p1, p2, p3, derivEps)
	}
	lastT := float32(0.0)
	lp0 := p0

	for {
		t0 := float32(t0u) * dt
		if t0 == 1.0 {
			break
		}
		t1 := t0 + dt
		thisP0 := lastP
		thisQ0 := lastQ
		thisP1, thisQ1 := evalCubicAndDeriv(p0, p1, p2, p3, t1)

		if thisQ1.lengthSq() < derivThresh*derivThresh {
			newP1, newQ1 := evalCubicAndDeriv(p0, p1, p2, p3, t1-derivEps)
			thisQ1 = newQ1
			if t1 < 1.0 {
				thisP1 = newP1
				t1 -= derivEps
			}
		}

		actualDt := t1 - lastT
		cp := cubicParamsFromPointsDerivs(thisP0, thisP1, thisQ0, thisQ1, actualDt)

		if cp.err <= flattenTol || dt <= subdivLimit {
			ep := eulerParamsFromAngles(cp.th0, cp.th1)
			es := eulerSeg{p0: thisP0, p1: thisP1, params: ep}

			k0MinusHalfK1 := es.params.k0 - 0.5*es.params.k1
			k1 := es.params.k1

			// Compute number of line subdivisions.
			// For fill (offset=0), normalizedOffset=0, distScaled=0.
			scaleMul := 0.5 * float32(math.Sqrt2/2.0) *
				float32(math.Sqrt(float64(cp.chordLen/(es.params.ch*flattenTol))))

			const k1Thresh float32 = 1e-3
			var nFrac float32
			var robust espcRobust
			var a, b, integral, int0 float32

			if abs32(k1) < k1Thresh {
				k := k0MinusHalfK1 + 0.5*k1
				nFrac = float32(math.Sqrt(float64(abs32(k))))
				robust = espcRobustLowK1
			} else {
				// distScaled=0 → LowDist path
				a = k1
				b = k0MinusHalfK1
				int0 = cubeSignedSqrt(b)
				int1 := cubeSignedSqrt(a + b)
				integral = int1 - int0
				nFrac = (2.0 / 3.0) * integral / a
				robust = espcRobustLowDist
			}

			n := float32(math.Ceil(float64(nFrac * scaleMul)))
			if n < 1 {
				n = 1
			}
			if n > 100 {
				n = 100
			}

			// Emit line segments.
			nInt := int(n)
			for i := range nInt {
				var lp1 vec2
				if i == nInt-1 && t1 == 1.0 {
					lp1 = p3 // Use exact endpoint
				} else {
					t := float32(i+1) / n
					var s float32
					switch robust {
					case espcRobustLowK1:
						s = t
					case espcRobustLowDist:
						c := float32(math.Cbrt(float64(integral*t + int0)))
						inv := c * abs32(c)
						s = (inv - b) / a
					case espcRobustNormal:
						inv := espcIntInvApprox(integral*t + int0)
						s = (inv - b) / a
					}
					lp1 = es.evalWithOffset(s, 0) // offset=0 for fill
				}
				*lines = append(*lines, LineSoup{
					P0: [2]float32{lp0.x, lp0.y},
					P1: [2]float32{lp1.x, lp1.y},
				})
				lp0 = lp1
			}

			lastP = thisP1
			lastQ = thisQ1
			lastT = t1

			// Advance: trailing zeros represent stack pops in adaptive subdivision.
			t0u++
			shift := trailingZeros32(t0u)
			t0u >>= shift
			dt *= float32(uint32(1) << shift)
		} else {
			// Subdivide: halve the range.
			if t0u < math.MaxUint32/2 {
				t0u *= 2
			}
			dt *= 0.5
		}
	}
}

type espcRobust int

const (
	espcRobustNormal espcRobust = iota
	espcRobustLowK1
	espcRobustLowDist
)

// cubeSignedSqrt computes x * |x|^0.5.
func cubeSignedSqrt(x float32) float32 {
	return x * float32(math.Sqrt(float64(abs32(x))))
}

func trailingZeros32(x uint32) uint {
	if x == 0 {
		return 32
	}
	var n uint
	for x&1 == 0 {
		n++
		x >>= 1
	}
	return n
}
