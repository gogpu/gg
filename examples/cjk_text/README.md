# CJK Text Rendering Validation

Validates CJK (Chinese/Japanese/Korean) text rendering quality (ADR-027).

## What it tests

- **Body text (12-24px)** — Tier 6 bitmap, exact-size rasterization (no bucket quantization)
- **Display text (36-72px)** — Tier 4 MSDF with 128px reference (dual atlas)
- **Mixed script** — Latin + CJK in the same line
- **TTC collections** — automatic .ttc font loading (msyh.ttc, simsun.ttc)

## Run

```bash
go run ./examples/cjk_text/
# Output: tmp/cjk_text_validation.png
```

## Expected Result

- All CJK characters sharp at all sizes (no blur from bucket scaling)
- Missing glyphs (□) only when font lacks coverage (e.g., Korean in Chinese-only font)
- Latin text quality unchanged

## Fonts

Uses first available CJK font:
- Windows: Microsoft YaHei (msyh.ttc), SimSun (simsun.ttc), Malgun Gothic
- macOS: PingFang (PingFang.ttc)
- Linux: Noto Sans CJK
