//go:build ggtest_vulkan

package gpu

// The GPU readback tests (TestDepthClipReadback) need a real backend in the test binary. wgpu selects backends by
// import (host-import model), so this tag registers Vulkan only: without a ggtest_* tag the binary keeps today's
// software/noop backends and CI behaves as before.
import _ "github.com/gogpu/wgpu/hal/vulkan"
