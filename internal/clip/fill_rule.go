package clip

// FillRule controls how a path is converted to a clip mask.
//
// The values intentionally mirror gg.FillRule without importing the parent
// package (which would create an import cycle).
type FillRule uint8

const (
	FillRuleNonZero FillRule = iota
	FillRuleEvenOdd
)
