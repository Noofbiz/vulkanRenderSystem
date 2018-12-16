package vulkanRenderSystem

import (
	vk "github.com/vulkan-go/vulkan"
)

// Cleanup cleans up all the vulkan memory used by the VulkanRenderSystem.
func (r *RenderSystem) Cleanup() {
	vk.DestroySemaphore(r.device, r.imageAvailableSemaphore, nil)
	vk.DestroySemaphore(r.device, r.renderFinishedSemaphore, nil)
	vk.DestroyCommandPool(r.device, r.commandPool, nil)
	for _, framebuffer := range r.swapChainFramebuffers {
		vk.DestroyFramebuffer(r.device, framebuffer, nil)
	}
	for _, pipeline := range r.graphicsPipelines {
		vk.DestroyPipeline(r.device, pipeline, nil)
	}
	vk.DestroyPipelineLayout(r.device, r.pipelineLayout, nil)
	vk.DestroyRenderPass(r.device, r.renderPass, nil)
	for _, view := range r.swapChainImageViews {
		vk.DestroyImageView(r.device, view, nil)
	}
	vk.DestroySwapchain(r.device, r.swapChain, nil)
	vk.DestroySurface(r.instance, r.surface, nil)
	vk.DestroyDevice(r.device, nil)
	vk.DestroyInstance(r.instance, nil)
}
