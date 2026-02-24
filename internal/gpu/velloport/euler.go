// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: BSD-3-Clause

// Port of vello_shaders/src/cpu/euler.rs — Euler Spiral math for curve flattening.
// Original: Copyright 2023 the Vello Authors, Apache-2.0 OR MIT OR Unlicense.

package velloport

import "math"

// tangentThresh is the threshold for tangents to be considered near zero length.
const tangentThresh float32 = 1e-6

// cubicParams contains parameters derived from a cubic Bezier for the purpose
// of fitting a G1 continuous Euler spiral segment and estimating the Frechet distance.
type cubicParams struct {
	th0      float32 // Tangent angle relative to chord at start
	th1      float32 // Tangent angle relative to chord at end
	chordLen float32 // Effective chord length, always robustly nonzero
	err      float32 // Estimated error between source cubic and proposed Euler spiral
}

// eulerParams contains the computed Euler spiral parameters.
type eulerParams struct {
	th0 float32
	k0  float32
	k1  float32
	ch  float32
}

// eulerSeg is an Euler spiral segment with endpoints and parameters.
type eulerSeg struct {
	p0     vec2
	p1     vec2
	params eulerParams
}

// cubicParamsFromPointsDerivs computes Euler spiral parameters from cubic endpoints and derivatives.
func cubicParamsFromPointsDerivs(p0, p1, q0, q1 vec2, dt float32) cubicParams {
	chord := p1.sub(p0)
	chordSq := chord.lengthSq()
	chordLen := float32(math.Sqrt(float64(chordSq)))

	// Near-zero chord: straight line case.
	if chordSq < tangentThresh*tangentThresh {
		chordErr := float32(math.Sqrt(float64((9.0/32.0)*(q0.lengthSq()+q1.lengthSq())))) * dt
		return cubicParams{th0: 0, th1: 0, chordLen: tangentThresh, err: chordErr}
	}

	scale := dt / chordSq
	h0 := vec2{
		q0.x*chord.x + q0.y*chord.y,
		q0.y*chord.x - q0.x*chord.y,
	}
	th0 := h0.atan2()
	d0 := h0.length() * scale

	h1 := vec2{
		q1.x*chord.x + q1.y*chord.y,
		q1.x*chord.y - q1.y*chord.x,
	}
	th1 := h1.atan2()
	d1 := h1.length() * scale

	cth0 := cos32(th0)
	cth1 := cos32(th1)
	var err float32
	if cth0*cth1 < 0 {
		err = 2.0
	} else {
		e0 := (2.0 / 3.0) / max32(1.0+cth0, 1e-9)
		e1 := (2.0 / 3.0) / max32(1.0+cth1, 1e-9)
		s0 := sin32(th0)
		s1 := sin32(th1)
		s01 := cth0*s1 + cth1*s0
		amin := 0.15 * (2*e0*s0 + 2*e1*s1 - e0*e1*s01)
		a := 0.15 * (2*d0*s0 + 2*d1*s1 - d0*d1*s01)
		aerr := abs32(a - amin)
		symm := abs32(th0 + th1)
		asymm := abs32(th0 - th1)
		dist := float32(math.Hypot(float64(d0-e0), float64(d1-e1)))
		ctr := 4.625e-6*pow32(symm, 5) + 7.5e-3*asymm*symm*symm
		haloSymm := 5e-3 * symm * dist
		haloAsymm := 7e-2 * asymm * dist
		err = ctr + 1.55*aerr + haloSymm + haloAsymm
	}
	err *= chordLen

	return cubicParams{th0: th0, th1: th1, chordLen: chordLen, err: err}
}

// eulerParamsFromAngles computes Euler spiral parameters from tangent angles.
func eulerParamsFromAngles(th0, th1 float32) eulerParams {
	k0 := th0 + th1
	dth := th1 - th0
	d2 := dth * dth
	k2 := k0 * k0

	// Compute k1 using polynomial approximation.
	a := float32(6.0)
	a -= d2 * (1.0 / 70.0)
	a -= (d2 * d2) * (1.0 / 10780.0)
	a += (d2 * d2 * d2) * 2.769178184818219e-07
	b := float32(-0.1) + d2*(1.0/4200.0) + d2*d2*1.6959677820260655e-05
	c := float32(-1.0/1400.0) + d2*6.84915970574303e-05 - k2*7.936475029053326e-06
	a += (b + c*k2) * k2
	k1 := dth * a

	// Compute chord length using polynomial approximation.
	ch := float32(1.0)
	ch -= d2 * (1.0 / 40.0)
	ch += (d2 * d2) * 0.00034226190482569864
	ch -= (d2 * d2 * d2) * 1.9349474568904524e-06
	b2 := float32(-1.0/24.0) + d2*0.0024702380951963226 - d2*d2*3.7297408997537985e-05
	c2 := float32(1.0/1920.0) - d2*4.87350869747975e-05 - k2*3.1001936068463107e-06
	ch += (b2 + c2*k2) * k2

	return eulerParams{th0: th0, k0: k0, k1: k1, ch: ch}
}

func (ep *eulerParams) evalTh(t float32) float32 {
	return (ep.k0+0.5*ep.k1*(t-1.0))*t - ep.th0
}

func (ep *eulerParams) eval(t float32) vec2 {
	thm := ep.evalTh(t * 0.5)
	u, v := integEuler10((ep.k0+ep.k1*(0.5*t-0.5))*t, ep.k1*t*t)
	s := t / ep.ch * sin32(thm)
	c := t / ep.ch * cos32(thm)
	return vec2{u*c - v*s, -v*c - u*s}
}

func (ep *eulerParams) evalWithOffset(t, offset float32) vec2 {
	th := ep.evalTh(t)
	ov := vec2{offset * sin32(th), offset * cos32(th)}
	return ep.eval(t).add(ov)
}

func (es *eulerSeg) evalWithOffset(t, normalizedOffset float32) vec2 {
	chord := es.p1.sub(es.p0)
	pt := es.params.evalWithOffset(t, normalizedOffset)
	return vec2{
		es.p0.x + chord.x*pt.x - chord.y*pt.y,
		es.p0.y + chord.x*pt.y + chord.y*pt.x,
	}
}

// integEuler10 integrates an Euler spiral using a 10th order polynomial.
func integEuler10(k0, k1 float32) (float32, float32) {
	t1_1 := k0
	t1_2 := 0.5 * k1
	t2_2 := t1_1 * t1_1
	t2_3 := 2.0 * (t1_1 * t1_2)
	t2_4 := t1_2 * t1_2
	t3_4 := t2_2*t1_2 + t2_3*t1_1
	t3_6 := t2_4 * t1_2
	t4_4 := t2_2 * t2_2
	t4_5 := 2.0 * (t2_2 * t2_3)
	t4_6 := 2.0*(t2_2*t2_4) + t2_3*t2_3
	t4_7 := 2.0 * (t2_3 * t2_4)
	t4_8 := t2_4 * t2_4
	t5_6 := t4_4*t1_2 + t4_5*t1_1
	t5_8 := t4_6*t1_2 + t4_7*t1_1
	t6_6 := t4_4 * t2_2
	t6_7 := t4_4*t2_3 + t4_5*t2_2
	t6_8 := t4_4*t2_4 + t4_5*t2_3 + t4_6*t2_2
	t7_8 := t6_6*t1_2 + t6_7*t1_1
	t8_8 := t6_6 * t2_2

	u := float32(1.0)
	u -= (1.0/24.0)*t2_2 + (1.0/160.0)*t2_4
	u += (1.0/1920.0)*t4_4 + (1.0/10752.0)*t4_6 + (1.0/55296.0)*t4_8
	u -= (1.0/322560.0)*t6_6 + (1.0/1658880.0)*t6_8
	u += (1.0 / 92897280.0) * t8_8

	v := (1.0 / 12.0) * t1_2
	v -= (1.0/480.0)*t3_4 + (1.0/2688.0)*t3_6
	v += (1.0/53760.0)*t5_6 + (1.0/276480.0)*t5_8
	v -= (1.0 / 11612160.0) * t7_8

	return u, v
}

// ESPC integral approximation constants.
const (
	espcBreak1 float32 = 0.8
	espcBreak2 float32 = 1.25
	espcBreak3 float32 = 2.1
	sinScale   float32 = 1.0976991822760038
	quadA1     float32 = 0.6406
	quadB1     float32 = -0.81
	quadC1     float32 = 0.9148117935952064
	quadA2     float32 = 0.5
	quadB2     float32 = -0.156
	quadC2     float32 = 0.16145779359520596
)

func espcIntApprox(x float32) float32 {
	y := abs32(x)
	var a float32
	if y < espcBreak1 {
		a = sin32(sinScale*y) * (1.0 / sinScale)
	} else if y < espcBreak2 {
		v := y - 1.0
		a = float32(math.Sqrt(8.0)/3.0)*v*float32(math.Sqrt(float64(abs32(v)))) + math.Pi/4.0
	} else {
		qa, qb, qc := quadA1, quadB1, quadC1
		if y >= espcBreak3 {
			qa, qb, qc = quadA2, quadB2, quadC2
		}
		a = qa*y*y + qb*y + qc
	}
	return copysign32(a, x)
}

func espcIntInvApprox(x float32) float32 {
	y := abs32(x)
	var a float32
	if y < 0.7010707591262915 {
		a = float32(math.Asin(float64(x*sinScale))) * (1.0 / sinScale)
	} else if y < 0.903249293595206 {
		b := y - math.Pi/4.0
		absB := abs32(b)
		u := copysign32(float32(math.Pow(float64(absB), 2.0/3.0)), b)
		a = u*float32(math.Cbrt(9.0/8.0)) + 1.0
	} else {
		var u, v, w float32
		if y < 2.038857793595206 {
			bv := 0.5 * quadB1 / quadA1
			u = bv*bv - quadC1/quadA1
			v = 1.0 / quadA1
			w = bv
		} else {
			bv := 0.5 * quadB2 / quadA2
			u = bv*bv - quadC2/quadA2
			v = 1.0 / quadA2
			w = bv
		}
		a = float32(math.Sqrt(float64(u+v*y))) - w
	}
	return copysign32(a, x)
}
