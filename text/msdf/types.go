package msdf

import (
	"math"
)

// Config holds MSDF generation parameters.
type Config struct {
	// Size is the output texture size (width = height).
	// Typical values: 32, 48, 64.
	// Default: 32
	Size int

	// Range is the distance range in pixels.
	// This controls how far from the edge the distance field extends.
	// Larger values = softer edges, smaller = sharper but less scalable.
	// Default: 4.0
	Range float64

	// AngleThreshold is the minimum angle (in radians) to consider a corner sharp.
	// Corners sharper than this get different channel colors to preserve them.
	// Default: pi/3 (60 degrees)
	AngleThreshold float64

	// EdgeThreshold is the minimum pseudo-distance for edge detection.
	// Used in edge coloring algorithm.
	// Default: 1.001
	EdgeThreshold float64
}

// DefaultConfig returns the default MSDF configuration.
// These values work well for most text rendering scenarios.
func DefaultConfig() Config {
	return Config{
		Size:           32,
		Range:          4.0,
		AngleThreshold: math.Pi / 3, // 60 degrees
		EdgeThreshold:  1.001,
	}
}

// Validate checks if the configuration is valid and returns an error if not.
func (c *Config) Validate() error {
	if c.Size < 8 {
		return &ConfigError{Field: "Size", Reason: "must be at least 8"}
	}
	if c.Size > 4096 {
		return &ConfigError{Field: "Size", Reason: "must be at most 4096"}
	}
	if c.Range <= 0 {
		return &ConfigError{Field: "Range", Reason: "must be positive"}
	}
	if c.AngleThreshold <= 0 || c.AngleThreshold > math.Pi {
		return &ConfigError{Field: "AngleThreshold", Reason: "must be in (0, pi]"}
	}
	if c.EdgeThreshold < 1 {
		return &ConfigError{Field: "EdgeThreshold", Reason: "must be at least 1.0"}
	}
	return nil
}

// ConfigError represents a configuration validation error.
type ConfigError struct {
	Field  string
	Reason string
}

func (e *ConfigError) Error() string {
	return "msdf: invalid config." + e.Field + ": " + e.Reason
}

// MSDF holds the generated multi-channel signed distance field.
type MSDF struct {
	// Data is the RGB pixel data (3 bytes per pixel, row-major order).
	// Red, Green, Blue channels encode directional distance.
	// The median of RGB gives the actual signed distance.
	Data []byte

	// Width of the texture in pixels.
	Width int

	// Height of the texture in pixels.
	Height int

	// Bounds is the expanded bounding box (shape bounds + pxRange padding)
	// in the original outline coordinate space.
	Bounds Rect

	// Scale is the scaling factor from outline coordinates to pixel coordinates.
	Scale float64

	// Translation offset from outline to texture coordinates.
	TranslateX, TranslateY float64
}

// PixelOffset returns the byte offset for pixel (x, y).
func (m *MSDF) PixelOffset(x, y int) int {
	return (y*m.Width + x) * 3
}

// SetPixel sets the RGB values at (x, y).
func (m *MSDF) SetPixel(x, y int, r, g, b byte) {
	offset := m.PixelOffset(x, y)
	m.Data[offset] = r
	m.Data[offset+1] = g
	m.Data[offset+2] = b
}

// GetPixel returns the RGB values at (x, y).
func (m *MSDF) GetPixel(x, y int) (r, g, b byte) {
	offset := m.PixelOffset(x, y)
	return m.Data[offset], m.Data[offset+1], m.Data[offset+2]
}

// OutlineToPixel converts outline coordinates to pixel coordinates.
func (m *MSDF) OutlineToPixel(ox, oy float64) (px, py float64) {
	px = (ox-m.Bounds.MinX)*m.Scale + m.TranslateX
	py = (oy-m.Bounds.MinY)*m.Scale + m.TranslateY
	return
}

// PixelToOutline converts pixel coordinates to outline coordinates.
func (m *MSDF) PixelToOutline(px, py float64) (ox, oy float64) {
	ox = (px-m.TranslateX)/m.Scale + m.Bounds.MinX
	oy = (py-m.TranslateY)/m.Scale + m.Bounds.MinY
	return
}

// Point represents a 2D point with float64 precision.
// Used internally for distance calculations.
type Point struct {
	X, Y float64
}

// Add returns p + q.
func (p Point) Add(q Point) Point {
	return Point{p.X + q.X, p.Y + q.Y}
}

// Sub returns p - q.
func (p Point) Sub(q Point) Point {
	return Point{p.X - q.X, p.Y - q.Y}
}

// Mul returns p * scalar.
func (p Point) Mul(s float64) Point {
	return Point{p.X * s, p.Y * s}
}

// Dot returns the dot product of p and q.
func (p Point) Dot(q Point) float64 {
	return p.X*q.X + p.Y*q.Y
}

// Cross returns the 2D cross product (z-component of 3D cross).
func (p Point) Cross(q Point) float64 {
	return p.X*q.Y - p.Y*q.X
}

// Length returns the Euclidean length of the vector.
func (p Point) Length() float64 {
	return math.Sqrt(p.X*p.X + p.Y*p.Y)
}

// LengthSquared returns the squared length (avoids sqrt).
func (p Point) LengthSquared() float64 {
	return p.X*p.X + p.Y*p.Y
}

// Normalized returns a unit vector in the same direction.
// Returns zero vector if length is zero.
func (p Point) Normalized() Point {
	length := p.Length()
	if length == 0 {
		return Point{}
	}
	return Point{p.X / length, p.Y / length}
}

// Perpendicular returns a perpendicular vector (rotated 90 degrees CCW).
func (p Point) Perpendicular() Point {
	return Point{-p.Y, p.X}
}

// Lerp returns linear interpolation between p and q: p + t*(q-p).
func (p Point) Lerp(q Point, t float64) Point {
	return Point{
		p.X + t*(q.X-p.X),
		p.Y + t*(q.Y-p.Y),
	}
}

// Angle returns the angle of the vector in radians (-pi, pi].
func (p Point) Angle() float64 {
	return math.Atan2(p.Y, p.X)
}

// AngleBetween returns the angle between two vectors in radians [0, pi].
func AngleBetween(a, b Point) float64 {
	dot := a.Dot(b)
	lenA := a.Length()
	lenB := b.Length()
	if lenA == 0 || lenB == 0 {
		return 0
	}
	cosAngle := dot / (lenA * lenB)
	// Clamp to [-1, 1] to handle floating point errors
	if cosAngle > 1 {
		cosAngle = 1
	}
	if cosAngle < -1 {
		cosAngle = -1
	}
	return math.Acos(cosAngle)
}

// Rect represents a 2D rectangle.
type Rect struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

// Width returns the width of the rectangle.
func (r Rect) Width() float64 {
	return r.MaxX - r.MinX
}

// Height returns the height of the rectangle.
func (r Rect) Height() float64 {
	return r.MaxY - r.MinY
}

// IsEmpty returns true if the rectangle has zero or negative area.
func (r Rect) IsEmpty() bool {
	return r.MinX >= r.MaxX || r.MinY >= r.MaxY
}

// Center returns the center point of the rectangle.
func (r Rect) Center() Point {
	return Point{
		(r.MinX + r.MaxX) / 2,
		(r.MinY + r.MaxY) / 2,
	}
}

// Contains returns true if the point is inside the rectangle.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.MinX && p.X <= r.MaxX && p.Y >= r.MinY && p.Y <= r.MaxY
}

// Expand returns a rectangle expanded by the given margin on all sides.
func (r Rect) Expand(margin float64) Rect {
	return Rect{
		MinX: r.MinX - margin,
		MinY: r.MinY - margin,
		MaxX: r.MaxX + margin,
		MaxY: r.MaxY + margin,
	}
}

// Union returns the smallest rectangle containing both r and s.
func (r Rect) Union(s Rect) Rect {
	return Rect{
		MinX: min(r.MinX, s.MinX),
		MinY: min(r.MinY, s.MinY),
		MaxX: max(r.MaxX, s.MaxX),
		MaxY: max(r.MaxY, s.MaxY),
	}
}

// SignedDistance represents a signed distance with additional metadata.
type SignedDistance struct {
	// Distance is the signed Euclidean distance.
	// Negative = inside, Positive = outside.
	Distance float64

	// Dot is the dot product used for resolving ambiguities.
	// Used when distances are equal.
	Dot float64
}

// NewSignedDistance creates a new signed distance.
func NewSignedDistance(distance, dot float64) SignedDistance {
	return SignedDistance{Distance: distance, Dot: dot}
}

// Infinite returns a signed distance representing infinity.
func Infinite() SignedDistance {
	return SignedDistance{Distance: math.MaxFloat64, Dot: 0}
}

// IsCloserThan returns true if d is closer to the edge than other.
func (d SignedDistance) IsCloserThan(other SignedDistance) bool {
	absD := math.Abs(d.Distance)
	absO := math.Abs(other.Distance)
	if absD < absO {
		return true
	}
	if absD > absO {
		return false
	}
	// Equal absolute distance - use dot product to break ties
	return d.Dot < other.Dot
}

// Combine returns the closer distance of the two.
func (d SignedDistance) Combine(other SignedDistance) SignedDistance {
	if d.IsCloserThan(other) {
		return d
	}
	return other
}
