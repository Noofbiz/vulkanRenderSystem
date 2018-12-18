package vulkanRenderSystem

import (
	vk "github.com/vulkan-go/vulkan"
)

// Cleanup cleans up all the vulkan memory used by the VulkanRenderSystem.
func (r *RenderSystem) Cleanup() {
	vk.DeviceWaitIdle(r.device)
	r.cleanupSwapChain()
	vk.DestroyBuffer(r.device, r.vertexBuffer, nil)
	vk.FreeMemory(r.device, r.vertexBufferMemory, nil)
	for i := 0; i < maxFramesInFlight; i++ {
		vk.DestroySemaphore(r.device, r.imageAvailableSemaphores[i], nil)
		vk.DestroySemaphore(r.device, r.renderFinishedSemaphores[i], nil)
		vk.DestroyFence(r.device, r.inFlightFences[i], nil)
	}
	vk.DestroyCommandPool(r.device, r.commandPool, nil)
	vk.DestroySurface(r.instance, r.surface, nil)
	vk.DestroyDevice(r.device, nil)
	vk.DestroyInstance(r.instance, nil)
}

func (r *RenderSystem) cleanupSwapChain() {
	for _, framebuffer := range r.swapChainFramebuffers {
		vk.DestroyFramebuffer(r.device, framebuffer, nil)
	}
	vk.FreeCommandBuffers(r.device, r.commandPool, uint32(len(r.commandBuffers)), r.commandBuffers)
	for _, pipeline := range r.graphicsPipelines {
		vk.DestroyPipeline(r.device, pipeline, nil)
	}
	vk.DestroyPipelineLayout(r.device, r.pipelineLayout, nil)
	vk.DestroyRenderPass(r.device, r.renderPass, nil)
	for _, view := range r.swapChainImageViews {
		vk.DestroyImageView(r.device, view, nil)
	}
	vk.DestroySwapchain(r.device, r.swapChain, nil)
}
