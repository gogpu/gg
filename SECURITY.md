# Security Policy

## Supported Versions

gogpu/gg is currently in early development (v0.x.x).

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |
| < 0.1.0 | :x:                |

## Reporting a Vulnerability

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please report security issues via:

1. **Private Security Advisory** (preferred):
   https://github.com/gogpu/gg/security/advisories/new

2. **GitHub Discussions** (for less critical issues):
   https://github.com/gogpu/gg/discussions

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact

### Response Timeline

- **Initial Response**: Within 72 hours
- **Fix & Disclosure**: Coordinated with reporter

## Security Considerations

gogpu/gg is a Pure Go 2D graphics library. Security considerations:

1. **Image Loading** - Be cautious with untrusted image files (PNG/JPG parsing)
2. **File System** - SavePNG/SaveJPG write to specified paths
3. **Memory** - Large canvases allocate significant memory

## Security Contact

- **GitHub Security Advisory**: https://github.com/gogpu/gg/security/advisories/new
- **Public Issues**: https://github.com/gogpu/gg/issues

---

**Thank you for helping keep gogpu/gg secure!**
