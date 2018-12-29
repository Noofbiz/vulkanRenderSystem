package vulkanRenderSystem

import (
	"unsafe"

	vk "github.com/vulkan-go/vulkan"
)

func clamp(high, low, value uint32) uint32 {
	var ret uint32
	if value > high {
		ret = high
	} else {
		ret = value
	}
	if ret < low {
		return low
	}
	return ret
}

var end = "\x00"
var endChar byte = '\x00'

func safeString(s string) string {
	if len(s) == 0 {
		return end
	}
	if s[len(s)-1] != endChar {
		return s + end
	}
	return s
}

func safeStrings(list []string) []string {
	for i := range list {
		list[i] = safeString(list[i])
	}
	return list
}

type swapChainSupportDetails struct {
	capabilities vk.SurfaceCapabilities
	formats      []vk.SurfaceFormat
	presentModes []vk.PresentMode
}

var details swapChainSupportDetails

func sliceUint32(data []byte) []uint32 {
	const m = 0x7fffffff
	return (*[m / 4]uint32)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&data)).Data))[:len(data)/4]
}

func vertexData(v vertex) []byte {
	const m = 0x7fffffff
	return (*[m]byte)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&v)).Data))[:len(v)*4]
}

func indexData(v []uint16) []byte {
	const m = 0x7fffffff
	return (*[m]byte)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&v)).Data))[:len(v)*2]
}

type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}
