//go:build !nogpu && gpuintegration

package gpu

// Register native backends only for explicit GPU integration runs.
import _ "github.com/gogpu/wgpu/hal/allbackends"
