package wide

import "testing"

// Benchmark U16x16 operations to verify SIMD auto-vectorization

func BenchmarkU16x16_Add(b *testing.B) {
	a := SplatU16(100)
	c := SplatU16(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Add(c)
	}
}

func BenchmarkU16x16_Sub(b *testing.B) {
	a := SplatU16(200)
	c := SplatU16(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Sub(c)
	}
}

func BenchmarkU16x16_Mul(b *testing.B) {
	a := SplatU16(100)
	c := SplatU16(2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Mul(c)
	}
}

func BenchmarkU16x16_Div255(b *testing.B) {
	a := SplatU16(12750) // 50 * 255
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Div255()
	}
}

func BenchmarkU16x16_Inv(b *testing.B) {
	a := SplatU16(128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Inv()
	}
}

func BenchmarkU16x16_MulDiv255(b *testing.B) {
	a := SplatU16(200)
	c := SplatU16(128)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.MulDiv255(c)
	}
}

func BenchmarkU16x16_Clamp(b *testing.B) {
	a := SplatU16(300)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Clamp(255)
	}
}

// Compare wide operations with scalar loops

func BenchmarkScalar_Add(b *testing.B) {
	var a, c, result [16]uint16
	for i := range a {
		a[i] = 100
		c[i] = 50
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range a {
			result[j] = a[j] + c[j]
		}
	}
}

func BenchmarkScalar_MulDiv255(b *testing.B) {
	var a, c, result [16]uint16
	for i := range a {
		a[i] = 200
		c[i] = 128
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range a {
			x := uint32(a[j]) * uint32(c[j])
			result[j] = uint16((x + 1 + (x >> 8)) >> 8)
		}
	}
}

// Benchmark F32x8 operations

func BenchmarkF32x8_Add(b *testing.B) {
	a := SplatF32(1.5)
	c := SplatF32(2.5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Add(c)
	}
}

func BenchmarkF32x8_Sub(b *testing.B) {
	a := SplatF32(10.0)
	c := SplatF32(3.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Sub(c)
	}
}

func BenchmarkF32x8_Mul(b *testing.B) {
	a := SplatF32(2.5)
	c := SplatF32(4.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Mul(c)
	}
}

func BenchmarkF32x8_Div(b *testing.B) {
	a := SplatF32(10.0)
	c := SplatF32(2.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Div(c)
	}
}

func BenchmarkF32x8_Sqrt(b *testing.B) {
	a := SplatF32(9.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Sqrt()
	}
}

func BenchmarkF32x8_Clamp(b *testing.B) {
	a := SplatF32(1.5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Clamp(0.0, 1.0)
	}
}

func BenchmarkF32x8_Lerp(b *testing.B) {
	a := SplatF32(0.0)
	c := SplatF32(10.0)
	t := SplatF32(0.5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Lerp(c, t)
	}
}

func BenchmarkF32x8_Min(b *testing.B) {
	a := SplatF32(3.0)
	c := SplatF32(7.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Min(c)
	}
}

func BenchmarkF32x8_Max(b *testing.B) {
	a := SplatF32(3.0)
	c := SplatF32(7.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.Max(c)
	}
}

// Benchmark BatchState operations

func BenchmarkBatchState_LoadSrc(b *testing.B) {
	src := make([]byte, 64)
	for i := range src {
		src[i] = uint8(i)
	}
	var batch BatchState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.LoadSrc(src)
	}
}

func BenchmarkBatchState_LoadDst(b *testing.B) {
	dst := make([]byte, 64)
	for i := range dst {
		dst[i] = uint8(i)
	}
	var batch BatchState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.LoadDst(dst)
	}
}

func BenchmarkBatchState_StoreDst(b *testing.B) {
	var batch BatchState
	for i := 0; i < 16; i++ {
		batch.DR[i] = uint16(i * 10)
		batch.DG[i] = uint16(i*10 + 1)
		batch.DB[i] = uint16(i*10 + 2)
		batch.DA[i] = uint16(i*10 + 3)
	}
	dst := make([]byte, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.StoreDst(dst)
	}
}

func BenchmarkBatchState_RoundTrip(b *testing.B) {
	src := make([]byte, 64)
	dst := make([]byte, 64)
	for i := range src {
		src[i] = uint8(i)
	}
	var batch BatchState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.LoadSrc(src)
		batch.DR = batch.SR
		batch.DG = batch.SG
		batch.DB = batch.SB
		batch.DA = batch.SA
		batch.StoreDst(dst)
	}
}

// Benchmark complete alpha blending operation using wide types

func BenchmarkBatch_AlphaBlend(b *testing.B) {
	src := make([]byte, 64)
	dst := make([]byte, 64)
	for i := range src {
		src[i] = uint8((i * 7) % 256)
		dst[i] = uint8((i * 13) % 256)
	}
	var batch BatchState
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.LoadSrc(src)
		batch.LoadDst(dst)

		// Perform alpha blending: dst = src + dst * (255 - alpha) / 255
		invAlpha := batch.SA.Inv()
		batch.DR = batch.SR.Add(batch.DR.MulDiv255(invAlpha))
		batch.DG = batch.SG.Add(batch.DG.MulDiv255(invAlpha))
		batch.DB = batch.SB.Add(batch.DB.MulDiv255(invAlpha))
		batch.DA = SplatU16(255)

		batch.StoreDst(dst)
	}
}

// Benchmark scalar version for comparison

func BenchmarkScalar_AlphaBlend(b *testing.B) {
	src := make([]byte, 64)
	dst := make([]byte, 64)
	for i := range src {
		src[i] = uint8((i * 7) % 256)
		dst[i] = uint8((i * 13) % 256)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 16; j++ {
			offset := j * 4
			sr := uint16(src[offset+0])
			sg := uint16(src[offset+1])
			sb := uint16(src[offset+2])
			sa := uint16(src[offset+3])
			dr := uint16(dst[offset+0])
			dg := uint16(dst[offset+1])
			db := uint16(dst[offset+2])

			invAlpha := 255 - sa
			dr = sr + uint16((uint32(dr)*uint32(invAlpha)+1+((uint32(dr)*uint32(invAlpha))>>8))>>8)
			dg = sg + uint16((uint32(dg)*uint32(invAlpha)+1+((uint32(dg)*uint32(invAlpha))>>8))>>8)
			db = sb + uint16((uint32(db)*uint32(invAlpha)+1+((uint32(db)*uint32(invAlpha))>>8))>>8)

			dst[offset+0] = uint8(dr)
			dst[offset+1] = uint8(dg)
			dst[offset+2] = uint8(db)
			dst[offset+3] = 255
		}
	}
}
