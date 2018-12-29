package vulkanRenderSystem

import (
	vk "github.com/vulkan-go/vulkan"
)

type vertex []float32

func (v *vertex) getBindingDescription() vk.VertexInputBindingDescription {
	return vk.VertexInputBindingDescription{
		Binding:   0,
		Stride:    5 * 4,
		InputRate: vk.VertexInputRateVertex,
	}
}

func (v *vertex) getAttributeDescriptions() []vk.VertexInputAttributeDescription {
	var a []vk.VertexInputAttributeDescription
	a = append(a, vk.VertexInputAttributeDescription{
		Binding:  0,
		Location: 0,
		Format:   vk.FormatR32g32Sfloat,
		Offset:   0,
	})
	a = append(a, vk.VertexInputAttributeDescription{
		Binding:  0,
		Location: 1,
		Format:   vk.FormatR32g32b32Sfloat,
		Offset:   2 * 4,
	})
	return a
}

var vertices = vertex{
	-0.5, -0.5, 1, 0, 0,
	0.5, -0.5, 0, 1, 0,
	0.5, 0.5, 0, 0, 1,
	-0.5, 0.5, 1, 1, 1,
}

var indices = []uint16{
	0, 1, 2, 2, 3, 0,
}
