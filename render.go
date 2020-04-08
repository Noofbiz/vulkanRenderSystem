package vulkanRenderSystem

import (
	"errors"
	"image/color"
	"log"
	"sync"
	"time"
	"unsafe"

	_ "image/jpeg"

	"github.com/EngoEngine/ecs"
	"github.com/EngoEngine/engo"
	"github.com/EngoEngine/systems/physics"

	"github.com/Noofbiz/vulkanRenderSystem/internal/shaders"
	"github.com/go-gl/mathgl/mgl32"

	vk "github.com/vulkan-go/vulkan"
)

// RenderSystemPriority is the priority of the render system. This makes sure rendering
// happens after all other systems in the update
const RenderSystemPriority = -1000

// maxFramesInFlight is the number of frames allowed to be in-flight. Places an
// upper-bound on the amount of frames drawn between draw calls
const maxFramesInFlight = 2

// RenderComponent is the component used by the RenderSystem
type RenderComponent struct {
	// Hidden is used to prevent drawing by OpenGL
	Hidden bool
	// Scale is the scale at which to render, in the X and Y axis. Not defining Scale, will default to engo.Point{1, 1}
	Scale engo.Point
	// Color defines how much of the color-components of the texture get used
	Color color.Color
	// Drawable refers to the Texture that should be drawn
	// Drawable Drawable
	// ZIndex is the drawing order for the entities
	Zindex int
}

type renderEntity struct {
	*ecs.BasicEntity
	*physics.SpaceComponent
	*RenderComponent
}

type RenderSystem struct {
	entities                 []renderEntity
	instance                 vk.Instance
	surface                  vk.Surface
	gpu                      vk.PhysicalDevice
	device                   vk.Device
	graphicsIdx              uint32
	graphicsQueue            vk.Queue
	presentIdx               uint32
	presentQueue             vk.Queue
	swapChain                vk.Swapchain
	images                   []vk.Image
	swapChainImageFormat     vk.Format
	swapChainExtent          vk.Extent2D
	swapChainImageViews      []vk.ImageView
	renderPass               vk.RenderPass
	pipelineLayout           vk.PipelineLayout
	graphicsPipelines        []vk.Pipeline
	swapChainFramebuffers    []vk.Framebuffer
	commandPool              vk.CommandPool
	commandBuffers           []vk.CommandBuffer
	imageAvailableSemaphores []vk.Semaphore
	renderFinishedSemaphores []vk.Semaphore
	inFlightFences           []vk.Fence
	currentFrame             int
	framebufferResized       bool
	lock                     sync.Mutex
	vertexBuffer             vk.Buffer
	vertexBufferMemory       vk.DeviceMemory
	indexBuffer              vk.Buffer
	indexBufferMemory        vk.DeviceMemory
	descriptorSetLayouts     []vk.DescriptorSetLayout
	uniformBuffers           []vk.Buffer
	uniformBuffersMemory     []vk.DeviceMemory
	startTime                time.Time
	descriptorPool           vk.DescriptorPool
	descriptorSets           []vk.DescriptorSet
}

var theRenderSystem *RenderSystem

func (r *RenderSystem) New(w *ecs.World) {
	if theRenderSystem != nil {
		log.Println("[VULKAN RENDER SYSTEM]: Render System already added.")
		return
	}
	theRenderSystem = r
	engo.Mailbox.Listen("WindowResizeMessage", func(m engo.Message) {
		_, ok := m.(engo.WindowResizeMessage)
		if !ok {
			return
		}
		r.lock.Lock()
		r.framebufferResized = true
		r.lock.Unlock()
	})
	if err := r.initVulkan(); err != nil {
		panic(err)
	}
	if err := r.createSwapChain(); err != nil {
		panic(err)
	}
	if err := r.createImageViews(); err != nil {
		panic(err)
	}
	if err := r.createRenderPass(); err != nil {
		panic(err)
	}
	if err := r.createDescriptorSetLayout(); err != nil {
		panic(err)
	}
	if err := r.createGraphicsPipeline(); err != nil {
		panic(err)
	}
	if err := r.createFrameBuffers(); err != nil {
		panic(err)
	}
	if err := r.createCommandPool(); err != nil {
		panic(err)
	}
	if err := r.createTextureImage(); err != nil {
		panic(err)
	}
	if err := r.createTextureImageView(); err != nil {
		panic(err)
	}
	if err := r.createTextureSampler(); err != nil {
		panic(err)
	}
	if err := r.createVertexBuffer(); err != nil {
		panic(err)
	}
	if err := r.createIndexBuffer(); err != nil {
		panic(err)
	}
	if err := r.createUniformBuffers(); err != nil {
		panic(err)
	}
	if err := r.createDescriptorPool(); err != nil {
		panic(err)
	}
	if err := r.createDescriptorSets(); err != nil {
		panic(err)
	}
	if err := r.createCommandBuffers(); err != nil {
		panic(err)
	}
	if err := r.createSyncObjects(); err != nil {
		panic(err)
	}
}

func (r *RenderSystem) Update(dt float32) {
	if r.startTime.IsZero() {
		r.startTime = time.Now()
	}
	var imageIndex uint32
	r.lock.Lock()
	if r.framebufferResized {
		r.recreateSwapChain()
		r.framebufferResized = false
		r.lock.Unlock()
		return
	}
	r.lock.Unlock()
	vk.WaitForFences(r.device, 1, r.inFlightFences[r.currentFrame:r.currentFrame], vk.True, vk.MaxUint64)
	if res := vk.AcquireNextImage(r.device, r.swapChain, vk.MaxUint64, r.imageAvailableSemaphores[r.currentFrame], vk.NullFence, &imageIndex); res != vk.Success {
		panic("failed to aquire swap chain image")
	}
	waitSemaphores := []vk.Semaphore{r.imageAvailableSemaphores[r.currentFrame]}
	waitStages := []vk.PipelineStageFlags{vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit)}
	signalSemaphores := []vk.Semaphore{r.renderFinishedSemaphores[r.currentFrame]}
	r.updateUniformBuffer(imageIndex)
	submitInfo := []vk.SubmitInfo{vk.SubmitInfo{
		SType:                vk.StructureTypeSubmitInfo,
		WaitSemaphoreCount:   1,
		PWaitSemaphores:      waitSemaphores,
		PWaitDstStageMask:    waitStages,
		CommandBufferCount:   1,
		PCommandBuffers:      []vk.CommandBuffer{r.commandBuffers[imageIndex]},
		SignalSemaphoreCount: 1,
		PSignalSemaphores:    signalSemaphores,
	}}
	vk.ResetFences(r.device, 1, r.inFlightFences[r.currentFrame:r.currentFrame])
	if vk.QueueSubmit(r.graphicsQueue, 1, submitInfo, r.inFlightFences[r.currentFrame]) != vk.Success {
		panic("failed to submit draw command buffer!")
	}
	presentInfo := vk.PresentInfo{
		SType:              vk.StructureTypePresentInfo,
		WaitSemaphoreCount: 1,
		PWaitSemaphores:    signalSemaphores,
		SwapchainCount:     1,
		PSwapchains:        []vk.Swapchain{r.swapChain},
		PImageIndices:      []uint32{imageIndex},
	}
	if res := vk.QueuePresent(r.presentQueue, &presentInfo); res != vk.Success {
		panic("failed to present draw")
	}
	r.currentFrame++
	r.currentFrame %= maxFramesInFlight
}

func (r *RenderSystem) Remove(e ecs.BasicEntity) {}

func (r *RenderSystem) initVulkan() error {
	version := engo.GetApplicationVersion()
	appInfo := vk.ApplicationInfo{
		SType:              vk.StructureTypeApplicationInfo,
		PApplicationName:   safeString(engo.GetTitle()),
		ApplicationVersion: vk.MakeVersion(version[0], version[1], version[2]),
		PEngineName:        safeString("engo engine"),
		EngineVersion:      vk.MakeVersion(1, 0, 2),
		ApiVersion:         vk.ApiVersion10,
	}
	wantedExtensions := []string{
		vk.KhrSwapchainExtensionName,
	}
	createInfo := vk.InstanceCreateInfo{}
	createInfo.SType = vk.StructureTypeInstanceCreateInfo
	createInfo.PApplicationInfo = &appInfo
	exts := engo.Window.GetRequiredInstanceExtensions()
	createInfo.EnabledExtensionCount = uint32(len(exts))
	createInfo.PpEnabledExtensionNames = exts
	if res := vk.CreateInstance(&createInfo, nil, &r.instance); res != vk.Success {
		return errors.New("unable to create vulkan instance")
	}
	if err := vk.InitInstance(r.instance); err != nil {
		return err
	}
	surfPtr, err := engo.Window.CreateWindowSurface(r.instance, nil)
	r.surface = vk.SurfaceFromPointer(surfPtr)
	if err != nil {
		return err
	}
	var deviceCount uint32
	if res := vk.EnumeratePhysicalDevices(r.instance, &deviceCount, nil); res != vk.Success {
		return errors.New("unable to get physical devices")
	}
	devices := make([]vk.PhysicalDevice, deviceCount)
	if res := vk.EnumeratePhysicalDevices(r.instance, &deviceCount, devices); res != vk.Success {
		return errors.New("unable to get physical devices")
	}
	var deviceSelected bool
	var physicalDevice vk.PhysicalDevice
deviceLoop:
	for _, device := range devices {
		var queueFamilyPropertyCount uint32
		var graphicsSupport, presentSupport bool
		vk.GetPhysicalDeviceQueueFamilyProperties(device, &queueFamilyPropertyCount, nil)
		if queueFamilyPropertyCount == 0 {
			continue
		}
		queueFamilyProperties := make([]vk.QueueFamilyProperties, queueFamilyPropertyCount)
		vk.GetPhysicalDeviceQueueFamilyProperties(device, &queueFamilyPropertyCount, queueFamilyProperties)
		for i, q := range queueFamilyProperties {
			q.Deref()
			if q.QueueFlags&vk.QueueFlags(vk.QueueGraphicsBit) != 0 {
				r.graphicsIdx = uint32(i)
				graphicsSupport = true
			}
			var b32PresentSupport vk.Bool32
			vk.GetPhysicalDeviceSurfaceSupport(device, uint32(i), r.surface, &b32PresentSupport)
			if b32PresentSupport.B() {
				presentSupport = true
				r.presentIdx = uint32(i)
			}
		}
		if !graphicsSupport || !presentSupport {
			continue
		}
		var extensionCount uint32
		vk.EnumerateDeviceExtensionProperties(device, "", &extensionCount, nil)
		if extensionCount == 0 {
			continue
		}
		availableExtensions := make([]vk.ExtensionProperties, extensionCount)
		vk.EnumerateDeviceExtensionProperties(device, "", &extensionCount, availableExtensions)
		for _, req := range wantedExtensions {
			extensionFound := false
			for _, ext := range availableExtensions {
				ext.Deref()
				if vk.ToString(ext.ExtensionName[:]) == req {
					extensionFound = true
					break
				}
			}
			if !extensionFound {
				continue deviceLoop
			}
		}
		if res := vk.GetPhysicalDeviceSurfaceCapabilities(device, r.surface, &details.capabilities); res != vk.Success {
			continue
		}
		var formatCount uint32
		vk.GetPhysicalDeviceSurfaceFormats(device, r.surface, &formatCount, nil)
		if formatCount == 0 {
			continue
		}
		details.formats = make([]vk.SurfaceFormat, formatCount)
		vk.GetPhysicalDeviceSurfaceFormats(device, r.surface, &formatCount, details.formats)
		var presentModeCount uint32
		vk.GetPhysicalDeviceSurfacePresentModes(device, r.surface, &presentModeCount, nil)
		if presentModeCount == 0 {
			continue
		}
		details.presentModes = make([]vk.PresentMode, presentModeCount)
		vk.GetPhysicalDeviceSurfacePresentModes(device, r.surface, &presentModeCount, details.presentModes)
		deviceSelected = true
		physicalDevice = device
		r.gpu = device
	}
	if !deviceSelected {
		return errors.New("failed to find a sutible GPU")
	}
	qi := []vk.DeviceQueueCreateInfo{{
		SType:            vk.StructureTypeDeviceQueueCreateInfo,
		QueueFamilyIndex: r.graphicsIdx,
		QueueCount:       1,
		PQueuePriorities: []float32{1.0},
	}}
	if r.graphicsIdx != r.presentIdx {
		qi = append(qi, vk.DeviceQueueCreateInfo{
			SType:            vk.StructureTypeDeviceQueueCreateInfo,
			QueueFamilyIndex: r.presentIdx,
			QueueCount:       1,
			PQueuePriorities: []float32{1.0},
		})
	}
	ret := vk.CreateDevice(physicalDevice, &vk.DeviceCreateInfo{
		SType:                   vk.StructureTypeDeviceCreateInfo,
		QueueCreateInfoCount:    uint32(len(qi)),
		PQueueCreateInfos:       qi,
		EnabledExtensionCount:   uint32(len(wantedExtensions)),
		PpEnabledExtensionNames: safeStrings(wantedExtensions),
	}, nil, &r.device)
	if ret != vk.Success {
		return errors.New("unable to create logical device")
	}
	vk.GetDeviceQueue(r.device, r.graphicsIdx, 0, &r.graphicsQueue)
	vk.GetDeviceQueue(r.device, r.presentIdx, 0, &r.presentQueue)
	return nil
}

func (r *RenderSystem) createSwapChain() error {
	surfaceFormat := r.chooseSwapSurfaceFormat()
	surfaceFormat.Deref()
	presentMode := r.chooseSwapPresentMode()
	extent := r.chooseSwapExtent()
	extent.Deref()
	details.capabilities.Deref()
	imageCount := details.capabilities.MinImageCount + 1
	if details.capabilities.MaxImageCount > 0 {
		if imageCount > details.capabilities.MaxImageCount {
			imageCount = details.capabilities.MaxImageCount
		}
	}
	createInfo := vk.SwapchainCreateInfo{
		SType:            vk.StructureTypeSwapchainCreateInfo,
		Surface:          r.surface,
		MinImageCount:    imageCount,
		ImageFormat:      surfaceFormat.Format,
		ImageColorSpace:  surfaceFormat.ColorSpace,
		ImageExtent:      extent,
		ImageArrayLayers: 1,
		ImageUsage:       vk.ImageUsageFlags(vk.ImageUsageColorAttachmentBit),
	}
	if r.graphicsIdx != r.presentIdx {
		createInfo.ImageSharingMode = vk.SharingModeConcurrent
		createInfo.QueueFamilyIndexCount = 2
		createInfo.PQueueFamilyIndices = []uint32{r.graphicsIdx, r.presentIdx}
	} else {
		createInfo.ImageSharingMode = vk.SharingModeExclusive
		createInfo.QueueFamilyIndexCount = 0
		createInfo.PQueueFamilyIndices = []uint32{}
	}
	createInfo.PreTransform = details.capabilities.CurrentTransform
	createInfo.CompositeAlpha = vk.CompositeAlphaOpaqueBit
	createInfo.PresentMode = presentMode
	createInfo.Clipped = vk.True
	createInfo.OldSwapchain = vk.Swapchain(vk.NullHandle)
	var swapchain vk.Swapchain
	if res := vk.CreateSwapchain(r.device, &createInfo, nil, &swapchain); res != vk.Success {
		return errors.New("failed to create swap chain")
	}
	r.swapChain = swapchain
	var numImgs uint32
	vk.GetSwapchainImages(r.device, r.swapChain, &numImgs, nil)
	r.images = make([]vk.Image, numImgs)
	if res := vk.GetSwapchainImages(r.device, r.swapChain, &numImgs, r.images); res != vk.Success {
		return errors.New("failed to get swap chain images")
	}
	r.swapChainImageFormat = surfaceFormat.Format
	r.swapChainExtent = extent
	return nil
}

func (r *RenderSystem) chooseSwapSurfaceFormat() vk.SurfaceFormat {
	if len(details.formats) == 1 {
		details.formats[0].Deref()
		if details.formats[0].Format == vk.FormatUndefined {
			return vk.SurfaceFormat{
				Format:     vk.FormatB8g8r8Unorm,
				ColorSpace: vk.ColorSpaceSrgbNonlinear,
			}
		}
	}
	for _, f := range details.formats {
		f.Deref()
		if f.Format == vk.FormatB8g8r8Unorm && f.ColorSpace == vk.ColorSpaceSrgbNonlinear {
			return f
		}
	}
	return details.formats[0]
}

func (r *RenderSystem) chooseSwapPresentMode() vk.PresentMode {
	bestMode := vk.PresentModeFifo
	for _, p := range details.presentModes {
		if p == vk.PresentModeMailbox {
			return p
		}
		if p == vk.PresentModeImmediate {
			bestMode = p
		}
	}
	return bestMode
}

func (r *RenderSystem) chooseSwapExtent() vk.Extent2D {
	details.capabilities.Deref()
	if details.capabilities.CurrentExtent.Width != vk.MaxUint32 {
		return details.capabilities.CurrentExtent
	}
	w := uint32(engo.CanvasWidth())
	h := uint32(engo.CanvasHeight())
	actualExtent := vk.Extent2D{
		Width:  w,
		Height: h,
	}
	actualExtent.Width = clamp(details.capabilities.MaxImageExtent.Width,
		details.capabilities.MinImageExtent.Width, actualExtent.Width)
	actualExtent.Height = clamp(details.capabilities.MaxImageExtent.Height,
		details.capabilities.MinImageExtent.Height, actualExtent.Height)
	return actualExtent
}

func (r *RenderSystem) createImageViews() error {
	r.swapChainImageViews = make([]vk.ImageView, len(r.images))
	for i, image := range r.images {
		createInfo := vk.ImageViewCreateInfo{
			SType:    vk.StructureTypeImageViewCreateInfo,
			Image:    image,
			ViewType: vk.ImageViewType2d,
			Format:   r.swapChainImageFormat,
		}
		createInfo.Components.R = vk.ComponentSwizzleIdentity
		createInfo.Components.G = vk.ComponentSwizzleIdentity
		createInfo.Components.B = vk.ComponentSwizzleIdentity
		createInfo.Components.A = vk.ComponentSwizzleIdentity
		createInfo.SubresourceRange.BaseMipLevel = 0
		createInfo.SubresourceRange.LevelCount = 1
		createInfo.SubresourceRange.BaseArrayLayer = 0
		createInfo.SubresourceRange.LayerCount = 1
		if res := vk.CreateImageView(r.device, &createInfo, nil, &r.swapChainImageViews[i]); res != vk.Success {
			return errors.New("unable to create image view from swap chain images")
		}
	}
	return nil
}

func (r *RenderSystem) createRenderPass() error {
	colorAttachment := vk.AttachmentDescription{
		Format:         r.swapChainImageFormat,
		Samples:        vk.SampleCount1Bit,
		LoadOp:         vk.AttachmentLoadOpClear,
		StoreOp:        vk.AttachmentStoreOpStore,
		StencilLoadOp:  vk.AttachmentLoadOpDontCare,
		StencilStoreOp: vk.AttachmentStoreOpDontCare,
		InitialLayout:  vk.ImageLayoutUndefined,
		FinalLayout:    vk.ImageLayoutPresentSrc,
	}

	colorAttachmentRef := vk.AttachmentReference{
		Attachment: 0,
		Layout:     vk.ImageLayoutColorAttachmentOptimal,
	}

	subpass := vk.SubpassDescription{
		PipelineBindPoint:    vk.PipelineBindPointGraphics,
		ColorAttachmentCount: 1,
		PColorAttachments:    []vk.AttachmentReference{colorAttachmentRef},
	}

	dependency := vk.SubpassDependency{
		SrcSubpass:    vk.SubpassExternal,
		DstSubpass:    0,
		SrcStageMask:  vk.PipelineStageFlags(vk.PipelineStageColorAttachmentOutputBit),
		SrcAccessMask: 0,
		DstStageMask:  vk.PipelineStageFlags(vk.AccessColorAttachmentReadBit | vk.AccessColorAttachmentWriteBit),
	}

	renderPassInfo := vk.RenderPassCreateInfo{
		SType:           vk.StructureTypeRenderPassCreateInfo,
		AttachmentCount: 1,
		PAttachments:    []vk.AttachmentDescription{colorAttachment},
		SubpassCount:    1,
		PSubpasses:      []vk.SubpassDescription{subpass},
		DependencyCount: 1,
		PDependencies:   []vk.SubpassDependency{dependency},
	}

	var renderPass vk.RenderPass
	if res := vk.CreateRenderPass(r.device, &renderPassInfo, nil, &renderPass); res != vk.Success {
		return errors.New("failed to create render pass")
	}
	r.renderPass = renderPass

	return nil
}

func (r *RenderSystem) createGraphicsPipeline() error {
	vertShaderData, err := shaders.Asset("vert.spv")
	if err != nil {
		return err
	}
	fragShaderData, err := shaders.Asset("frag.spv")
	if err != nil {
		return err
	}
	vertShaderModule, err := r.loadShaderModule(vertShaderData)
	if err != nil {
		return err
	}
	fragShaderModule, err := r.loadShaderModule(fragShaderData)
	if err != nil {
		return err
	}

	vertShaderStageInfo := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageVertexBit,
		Module: vertShaderModule,
		PName:  safeString("main"),
	}

	fragShaderStageInfo := vk.PipelineShaderStageCreateInfo{
		SType:  vk.StructureTypePipelineShaderStageCreateInfo,
		Stage:  vk.ShaderStageFragmentBit,
		Module: fragShaderModule,
		PName:  safeString("main"),
	}

	shaderStages := []vk.PipelineShaderStageCreateInfo{
		vertShaderStageInfo,
		fragShaderStageInfo,
	}

	a := vertices.getAttributeDescriptions()
	b := vertices.getBindingDescription()

	vertexInputInfo := vk.PipelineVertexInputStateCreateInfo{
		SType:                           vk.StructureTypePipelineVertexInputStateCreateInfo,
		VertexBindingDescriptionCount:   1,
		VertexAttributeDescriptionCount: uint32(len(a)),
		PVertexBindingDescriptions:      []vk.VertexInputBindingDescription{b},
		PVertexAttributeDescriptions:    a,
	}

	inputAssembly := vk.PipelineInputAssemblyStateCreateInfo{
		SType:                  vk.StructureTypePipelineInputAssemblyStateCreateInfo,
		Topology:               vk.PrimitiveTopologyTriangleList,
		PrimitiveRestartEnable: vk.False,
	}

	viewport := vk.Viewport{
		X:        0,
		Y:        0,
		Width:    float32(r.swapChainExtent.Width),
		Height:   float32(r.swapChainExtent.Height),
		MinDepth: 0,
		MaxDepth: 1,
	}

	scissor := vk.Rect2D{
		Offset: vk.Offset2D{
			X: 0,
			Y: 0,
		},
		Extent: r.swapChainExtent,
	}

	viewportState := vk.PipelineViewportStateCreateInfo{
		SType:         vk.StructureTypePipelineViewportStateCreateInfo,
		ViewportCount: 1,
		PViewports:    []vk.Viewport{viewport},
		ScissorCount:  1,
		PScissors:     []vk.Rect2D{scissor},
	}

	rasterizer := vk.PipelineRasterizationStateCreateInfo{
		SType:                   vk.StructureTypePipelineRasterizationStateCreateInfo,
		DepthClampEnable:        vk.False,
		RasterizerDiscardEnable: vk.False,
		PolygonMode:             vk.PolygonModeFill,
		LineWidth:               1,
		CullMode:                vk.CullModeFlags(vk.CullModeBackBit),
		FrontFace:               vk.FrontFaceCounterClockwise,
		DepthBiasEnable:         vk.False,
	}

	multisampling := vk.PipelineMultisampleStateCreateInfo{
		SType:                 vk.StructureTypePipelineMultisampleStateCreateInfo,
		SampleShadingEnable:   vk.False,
		RasterizationSamples:  vk.SampleCount1Bit,
		MinSampleShading:      1,
		AlphaToCoverageEnable: vk.False,
		AlphaToOneEnable:      vk.False,
	}

	colorBlendAttachment := vk.PipelineColorBlendAttachmentState{
		ColorWriteMask:      vk.ColorComponentFlags(vk.ColorComponentRBit | vk.ColorComponentGBit | vk.ColorComponentBBit | vk.ColorComponentABit),
		BlendEnable:         vk.False,
		SrcColorBlendFactor: vk.BlendFactorOne,
		DstColorBlendFactor: vk.BlendFactorZero,
		ColorBlendOp:        vk.BlendOpAdd,
		SrcAlphaBlendFactor: vk.BlendFactorOne,
		DstAlphaBlendFactor: vk.BlendFactorZero,
		AlphaBlendOp:        vk.BlendOpAdd,
	}

	colorBlending := vk.PipelineColorBlendStateCreateInfo{
		SType:           vk.StructureTypePipelineColorBlendStateCreateInfo,
		LogicOpEnable:   vk.False,
		AttachmentCount: 1,
		PAttachments:    []vk.PipelineColorBlendAttachmentState{colorBlendAttachment},
	}

	pipelineLayoutInfo := vk.PipelineLayoutCreateInfo{
		SType:          vk.StructureTypePipelineLayoutCreateInfo,
		SetLayoutCount: 1,
		PSetLayouts:    r.descriptorSetLayouts,
	}
	var pipelineLayout vk.PipelineLayout
	if res := vk.CreatePipelineLayout(r.device, &pipelineLayoutInfo, nil, &pipelineLayout); res != vk.Success {
		return errors.New("failed to create pipeline layout")
	}
	r.pipelineLayout = pipelineLayout

	pipelineInfo := vk.GraphicsPipelineCreateInfo{
		SType:               vk.StructureTypeGraphicsPipelineCreateInfo,
		StageCount:          2,
		PStages:             shaderStages,
		PVertexInputState:   &vertexInputInfo,
		PInputAssemblyState: &inputAssembly,
		PViewportState:      &viewportState,
		PRasterizationState: &rasterizer,
		PMultisampleState:   &multisampling,
		PColorBlendState:    &colorBlending,
		Layout:              r.pipelineLayout,
		RenderPass:          r.renderPass,
		Subpass:             0,
	}

	r.graphicsPipelines = make([]vk.Pipeline, 1)
	if res := vk.CreateGraphicsPipelines(r.device, nil, 1, []vk.GraphicsPipelineCreateInfo{pipelineInfo}, nil, r.graphicsPipelines); res != vk.Success {
		errors.New("failed to create graphics pipeline")
	}

	vk.DestroyShaderModule(r.device, vertShaderModule, nil)
	vk.DestroyShaderModule(r.device, fragShaderModule, nil)

	return nil
}

func (r *RenderSystem) loadShaderModule(data []byte) (vk.ShaderModule, error) {
	var module vk.ShaderModule
	if res := vk.CreateShaderModule(r.device, &vk.ShaderModuleCreateInfo{
		SType:    vk.StructureTypeShaderModuleCreateInfo,
		CodeSize: uint(len(data)),
		PCode:    sliceUint32(data),
	}, nil, &module); res != vk.Success {
		return vk.NullShaderModule, errors.New("unable to create shader module")
	}
	return module, nil
}

func (r *RenderSystem) createFrameBuffers() error {
	r.swapChainFramebuffers = make([]vk.Framebuffer, len(r.swapChainImageViews))

	for idx, view := range r.swapChainImageViews {
		attachments := []vk.ImageView{view}

		framebufferInfo := vk.FramebufferCreateInfo{
			SType:           vk.StructureTypeFramebufferCreateInfo,
			RenderPass:      r.renderPass,
			AttachmentCount: 1,
			PAttachments:    attachments,
			Width:           r.swapChainExtent.Width,
			Height:          r.swapChainExtent.Height,
			Layers:          1,
		}

		if res := vk.CreateFramebuffer(r.device, &framebufferInfo, nil, &r.swapChainFramebuffers[idx]); res != vk.Success {
			return errors.New("failed to create framebuffer")
		}
	}

	return nil
}

func (r *RenderSystem) createCommandPool() error {
	poolInfo := vk.CommandPoolCreateInfo{
		SType:            vk.StructureTypeCommandPoolCreateInfo,
		QueueFamilyIndex: r.graphicsIdx,
	}

	var commandPool vk.CommandPool
	if res := vk.CreateCommandPool(r.device, &poolInfo, nil, &commandPool); res != vk.Success {
		return errors.New("failed to create command pool")
	}
	r.commandPool = commandPool

	return nil
}

func (r *RenderSystem) createTextureImage() error {
	for _, url := range imagesToAdd {
		err := engo.Files.Load(url)
		if err != nil {
			return err
		}
	}
	imagesToAdd = make([]string, 0)
	return nil
}

func (r *RenderSystem) createTextureImageView() error {
	return nil
}

func (r *RenderSystem) createTextureSampler() error {
	return nil
}

func (r *RenderSystem) createCommandBuffers() error {
	r.commandBuffers = make([]vk.CommandBuffer, len(r.swapChainFramebuffers))

	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		CommandPool:        r.commandPool,
		Level:              vk.CommandBufferLevelPrimary,
		CommandBufferCount: uint32(len(r.commandBuffers)),
	}

	if res := vk.AllocateCommandBuffers(r.device, &allocInfo, r.commandBuffers); res != vk.Success {
		errors.New("failed to allocate command buffers")
	}

	for idx, buffer := range r.commandBuffers {
		beginInfo := vk.CommandBufferBeginInfo{
			SType: vk.StructureTypeCommandBufferBeginInfo,
			Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageSimultaneousUseBit),
		}
		if res := vk.BeginCommandBuffer(buffer, &beginInfo); res != vk.Success {
			return errors.New("failed to begin recording command buffers")
		}
		clearValue := vk.NewClearValue([]float32{0, 0, 0, 1})
		renderPassInfo := vk.RenderPassBeginInfo{
			SType:           vk.StructureTypeRenderPassBeginInfo,
			RenderPass:      r.renderPass,
			Framebuffer:     r.swapChainFramebuffers[idx],
			ClearValueCount: 1,
			PClearValues:    []vk.ClearValue{clearValue},
		}
		renderPassInfo.RenderArea.Offset = vk.Offset2D{X: 0, Y: 0}
		renderPassInfo.RenderArea.Extent = r.swapChainExtent
		vk.CmdBeginRenderPass(buffer, &renderPassInfo, vk.SubpassContentsInline)
		vk.CmdBindPipeline(buffer, vk.PipelineBindPointGraphics, r.graphicsPipelines[0])
		buffers := []vk.Buffer{r.vertexBuffer}
		offsets := []vk.DeviceSize{0}
		vk.CmdBindVertexBuffers(buffer, 0, 1, buffers, offsets)
		vk.CmdBindIndexBuffer(buffer, r.indexBuffer, 0, vk.IndexTypeUint16)
		vk.CmdBindDescriptorSets(buffer, vk.PipelineBindPointGraphics, r.pipelineLayout, 0, 1, r.descriptorSets, 0, nil)
		vk.CmdDrawIndexed(buffer, uint32(len(indices)), 1, 0, 0, 0)
		vk.CmdEndRenderPass(buffer)
		if vk.EndCommandBuffer(buffer) != vk.Success {
			return errors.New("failed to record command buffer!")
		}
	}

	return nil
}

func (r *RenderSystem) createSyncObjects() error {
	r.imageAvailableSemaphores = make([]vk.Semaphore, maxFramesInFlight)
	r.renderFinishedSemaphores = make([]vk.Semaphore, maxFramesInFlight)
	r.inFlightFences = make([]vk.Fence, maxFramesInFlight)

	semaphoreInfo := vk.SemaphoreCreateInfo{
		SType: vk.StructureTypeSemaphoreCreateInfo,
	}

	fenceInfo := vk.FenceCreateInfo{
		SType: vk.StructureTypeFenceCreateInfo,
		Flags: vk.FenceCreateFlags(vk.FenceCreateSignaledBit),
	}

	for i := 0; i < maxFramesInFlight; i++ {
		var imageAvailableSemaphore vk.Semaphore
		if vk.CreateSemaphore(r.device, &semaphoreInfo, nil, &imageAvailableSemaphore) != vk.Success {
			return errors.New("failed to create image semaphore")
		}
		r.imageAvailableSemaphores[i] = imageAvailableSemaphore

		var renderFinishedSemaphore vk.Semaphore
		if vk.CreateSemaphore(r.device, &semaphoreInfo, nil, &renderFinishedSemaphore) != vk.Success {
			return errors.New("failed to create render finished semaphore")
		}
		r.renderFinishedSemaphores[i] = renderFinishedSemaphore

		var fence vk.Fence
		if vk.CreateFence(r.device, &fenceInfo, nil, &fence) != vk.Success {
			return errors.New("failed to create fence")
		}
		r.inFlightFences[i] = fence
	}

	return nil
}

func (r *RenderSystem) recreateSwapChain() error {
	vk.DeviceWaitIdle(r.device)

	r.cleanupSwapChain()

	if err := r.createSwapChain(); err != nil {
		return err
	}
	if err := r.createImageViews(); err != nil {
		return err
	}
	if err := r.createRenderPass(); err != nil {
		return err
	}
	if err := r.createGraphicsPipeline(); err != nil {
		return err
	}
	if err := r.createFrameBuffers(); err != nil {
		return err
	}
	if err := r.createCommandBuffers(); err != nil {
		return err
	}
	return nil
}

func (r *RenderSystem) createVertexBuffer() error {
	var err error
	bufferSize := vk.DeviceSize(4 * uint64(len(vertices)))
	stagingBuffer, stagingBufferMemory, err := r.createBuffer(bufferSize, vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit), vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		return err
	}

	var data unsafe.Pointer
	vk.MapMemory(r.device, stagingBufferMemory, 0, bufferSize, 0, &data)
	n := vk.Memcopy(data, vertexData(vertices))
	if n != len(vertices)*4 {
		return errors.New("failed to copy vertex buffer data")
	}
	vk.UnmapMemory(r.device, stagingBufferMemory)

	r.vertexBuffer, r.vertexBufferMemory, err = r.createBuffer(bufferSize, vk.BufferUsageFlags(vk.BufferUsageTransferDstBit|vk.BufferUsageVertexBufferBit), vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		return err
	}

	r.copyBuffer(stagingBuffer, r.vertexBuffer, bufferSize)

	vk.DestroyBuffer(r.device, stagingBuffer, nil)
	vk.FreeMemory(r.device, stagingBufferMemory, nil)

	return nil
}

func (r *RenderSystem) findMemoryType(typeFilter uint32, properties vk.MemoryPropertyFlags) (uint32, error) {
	memProp := vk.PhysicalDeviceMemoryProperties{}
	vk.GetPhysicalDeviceMemoryProperties(r.gpu, &memProp)
	memProp.Deref()
	for i := uint32(0); i < memProp.MemoryTypeCount; i++ {
		memProp.MemoryTypes[i].Deref()
		if typeFilter&(1<<i) != 0 && (memProp.MemoryTypes[i].PropertyFlags&properties) == properties {
			return i, nil
		}
	}
	return 0, errors.New("failed to find suitable memory type")
}

func (r *RenderSystem) createBuffer(size vk.DeviceSize, usage vk.BufferUsageFlags, properties vk.MemoryPropertyFlags) (buffer vk.Buffer, bufferMemory vk.DeviceMemory, err error) {
	bufferCreateInfo := vk.BufferCreateInfo{
		SType:       vk.StructureTypeBufferCreateInfo,
		Size:        size,
		Usage:       usage,
		SharingMode: vk.SharingModeExclusive,
	}
	if res := vk.CreateBuffer(r.device, &bufferCreateInfo, nil, &buffer); res != vk.Success {
		err = errors.New("unable to create buffer")
		return
	}

	memReq := vk.MemoryRequirements{}
	vk.GetBufferMemoryRequirements(r.device, buffer, &memReq)
	memReq.Deref()

	memIdx, err := r.findMemoryType(memReq.MemoryTypeBits, properties)
	if err != nil {
		return
	}
	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memReq.Size,
		MemoryTypeIndex: memIdx,
	}
	if res := vk.AllocateMemory(r.device, &allocInfo, nil, &bufferMemory); res != vk.Success {
		err = errors.New("failed to allocate buffer memory")
		return
	}

	if res := vk.BindBufferMemory(r.device, buffer, bufferMemory, 0); res != vk.Success {
		err = errors.New("unable to bind buffer memory")
		return
	}

	return
}

func (r *RenderSystem) copyBuffer(src, dst vk.Buffer, size vk.DeviceSize) error {
	commandBufs, err := r.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	copyRegion := []vk.BufferCopy{vk.BufferCopy{
		SrcOffset: 0,
		DstOffset: 0,
		Size:      size,
	}}
	vk.CmdCopyBuffer(commandBufs[0], src, dst, 1, copyRegion)

	return r.endSingleTimeCommands(commandBufs)
}

func (r *RenderSystem) beginSingleTimeCommands() ([]vk.CommandBuffer, error) {
	allocInfo := vk.CommandBufferAllocateInfo{
		SType:              vk.StructureTypeCommandBufferAllocateInfo,
		Level:              vk.CommandBufferLevelPrimary,
		CommandPool:        r.commandPool,
		CommandBufferCount: 1,
	}
	var commandBuf vk.CommandBuffer
	commandBufs := []vk.CommandBuffer{commandBuf}
	if res := vk.AllocateCommandBuffers(r.device, &allocInfo, commandBufs); res != vk.Success {
		return commandBufs, errors.New("unable to allocate command buffer at copy buffer")
	}

	beginInfo := vk.CommandBufferBeginInfo{
		SType: vk.StructureTypeCommandBufferBeginInfo,
		Flags: vk.CommandBufferUsageFlags(vk.CommandBufferUsageOneTimeSubmitBit),
	}
	if res := vk.BeginCommandBuffer(commandBufs[0], &beginInfo); res != vk.Success {
		return commandBufs, errors.New("unable to begin command buffer at copy buffer")
	}
	return commandBufs, nil
}

func (r *RenderSystem) endSingleTimeCommands(commandBufs []vk.CommandBuffer) error {
	if res := vk.EndCommandBuffer(commandBufs[0]); res != vk.Success {
		return errors.New("unable to end command buffer")
	}

	submitInfo := vk.SubmitInfo{
		SType:              vk.StructureTypeSubmitInfo,
		CommandBufferCount: 1,
		PCommandBuffers:    commandBufs,
	}

	if res := vk.QueueSubmit(r.graphicsQueue, 1, []vk.SubmitInfo{submitInfo}, vk.NullFence); res != vk.Success {
		return errors.New("unable to submit command to queue at copy buffer")
	}
	vk.QueueWaitIdle(r.graphicsQueue)

	vk.FreeCommandBuffers(r.device, r.commandPool, 1, commandBufs)

	return nil
}

func (r *RenderSystem) transitionImageLayout(image vk.Image, format vk.Format, oldLayout, newLayout vk.ImageLayout) error {
	commandBuffers, err := r.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	barrier := []vk.ImageMemoryBarrier{
		vk.ImageMemoryBarrier{
			SType:               vk.StructureTypeImageMemoryBarrier,
			OldLayout:           oldLayout,
			NewLayout:           newLayout,
			SrcQueueFamilyIndex: vk.QueueFamilyIgnored,
			DstQueueFamilyIndex: vk.QueueFamilyIgnored,
			Image:               image,
			SubresourceRange: vk.ImageSubresourceRange{
				AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
				BaseMipLevel:   0,
				LevelCount:     1,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			SrcAccessMask: vk.AccessFlags(0),
			DstAccessMask: vk.AccessFlags(0),
		},
	}

	vk.CmdPipelineBarrier(commandBuffers[0], 0, 0, 0, 0, nil, 0, nil, 0, barrier)

	return r.endSingleTimeCommands(commandBuffers)
}

func (r *RenderSystem) copyBufferToImage(buffer vk.Buffer, image vk.Image, width, height uint32) error {
	commandBuffers, err := r.beginSingleTimeCommands()
	if err != nil {
		return err
	}

	regions := []vk.BufferImageCopy{
		vk.BufferImageCopy{
			BufferOffset:      vk.DeviceSize(0),
			BufferRowLength:   0,
			BufferImageHeight: 0,
			ImageSubresource: vk.ImageSubresourceLayers{
				AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
				MipLevel:       0,
				BaseArrayLayer: 0,
				LayerCount:     1,
			},
			ImageOffset: vk.Offset3D{X: 0, Y: 0, Z: 0},
			ImageExtent: vk.Extent3D{
				Width:  width,
				Height: height,
				Depth:  1,
			},
		},
	}

	vk.CmdCopyBufferToImage(commandBuffers[0], buffer, image, vk.ImageLayoutTransferDstOptimal, 1, regions)

	return r.endSingleTimeCommands(commandBuffers)
}

func (r *RenderSystem) createIndexBuffer() error {
	var err error
	bufferSize := vk.DeviceSize(2 * uint64(len(indices)))
	stagingBuffer, stagingBufferMemory, err := r.createBuffer(bufferSize, vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit), vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		return err
	}

	var data unsafe.Pointer
	vk.MapMemory(r.device, stagingBufferMemory, 0, bufferSize, 0, &data)
	n := vk.Memcopy(data, indexData(indices))
	if n != len(indices)*2 {
		return errors.New("failed to copy index buffer data")
	}
	vk.UnmapMemory(r.device, stagingBufferMemory)

	r.indexBuffer, r.indexBufferMemory, err = r.createBuffer(bufferSize, vk.BufferUsageFlags(vk.BufferUsageTransferDstBit|vk.BufferUsageIndexBufferBit), vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		return err
	}

	r.copyBuffer(stagingBuffer, r.indexBuffer, bufferSize)

	vk.DestroyBuffer(r.device, stagingBuffer, nil)
	vk.FreeMemory(r.device, stagingBufferMemory, nil)

	return nil
}

func (r *RenderSystem) createDescriptorSetLayout() error {
	uboLayoutBinding := vk.DescriptorSetLayoutBinding{
		Binding:            0,
		DescriptorType:     vk.DescriptorTypeUniformBuffer,
		DescriptorCount:    1,
		StageFlags:         vk.ShaderStageFlags(vk.ShaderStageVertexBit),
		PImmutableSamplers: []vk.Sampler{vk.NullSampler},
	}
	samplerLayoutBinding := vk.DescriptorSetLayoutBinding{
		Binding:         1,
		DescriptorCount: 1,
		DescriptorType:  vk.DescriptorTypeCombinedImageSampler,
		StageFlags:      vk.ShaderStageFlags(vk.ShaderStageFragmentBit),
	}
	bindings := []vk.DescriptorSetLayoutBinding{uboLayoutBinding, samplerLayoutBinding}

	layoutInfo := vk.DescriptorSetLayoutCreateInfo{
		SType:        vk.StructureTypeDescriptorSetLayoutCreateInfo,
		BindingCount: uint32(len(bindings)),
		PBindings:    bindings,
	}

	var descriptorSetLayout vk.DescriptorSetLayout
	if res := vk.CreateDescriptorSetLayout(r.device, &layoutInfo, nil, &descriptorSetLayout); res != vk.Success {
		return errors.New("unable to create descriptor set layout")
	}
	r.descriptorSetLayouts = append(r.descriptorSetLayouts, descriptorSetLayout)

	return nil
}

func (r *RenderSystem) createUniformBuffers() error {
	bufferSize := vk.DeviceSize(4 * 16 * 3)
	var err error

	numImages := len(r.images)
	r.uniformBuffers = make([]vk.Buffer, numImages)
	r.uniformBuffersMemory = make([]vk.DeviceMemory, numImages)

	for i := 0; i < numImages; i++ {
		r.uniformBuffers[i], r.uniformBuffersMemory[i], err = r.createBuffer(bufferSize,
			vk.BufferUsageFlags(vk.BufferUsageUniformBufferBit),
			vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *RenderSystem) updateUniformBuffer(currentImageIdx uint32) error {
	elapsed := time.Now().Sub(r.startTime)
	ubo := UniformBufferObject{
		model:      mgl32.Ident4(),
		view:       mgl32.Ident4(),
		projection: mgl32.Ident4(),
	}
	ubo.model = ubo.model.Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(90) * float32(elapsed.Seconds())))
	ubo.view = ubo.view.Mul4(mgl32.LookAt(2, 2, 2, 0, 0, 0, 0, 0, 1))
	aspect := float32(r.swapChainExtent.Width) / float32(r.swapChainExtent.Height)
	ubo.projection = ubo.projection.Mul4(mgl32.Perspective(mgl32.DegToRad(45), aspect, 0.1, 10))
	ubo.projection.Set(1, 1, ubo.projection.At(1, 1)*-1)

	var data unsafe.Pointer
	bufferSize := vk.DeviceSize(4 * 16 * 3)
	vk.MapMemory(r.device, r.uniformBuffersMemory[currentImageIdx], 0, bufferSize, 0, &data)
	n := vk.Memcopy(data, uniformData(ubo))
	if n != 4*16*3 {
		return errors.New("failed to copy vertex buffer data")
	}
	vk.UnmapMemory(r.device, r.uniformBuffersMemory[currentImageIdx])
	return nil
}

func (r *RenderSystem) createDescriptorPool() error {
	poolSize := vk.DescriptorPoolSize{
		Type:            vk.DescriptorTypeUniformBuffer,
		DescriptorCount: uint32(len(r.images)),
	}
	imgSamplerSize := vk.DescriptorPoolSize{
		Type:            vk.DescriptorTypeCombinedImageSampler,
		DescriptorCount: uint32(len(r.images)),
	}
	poolSizes := []vk.DescriptorPoolSize{poolSize, imgSamplerSize}
	poolInfo := vk.DescriptorPoolCreateInfo{
		SType:         vk.StructureTypeDescriptorPoolCreateInfo,
		PoolSizeCount: uint32(len(poolSizes)),
		PPoolSizes:    poolSizes,
		MaxSets:       uint32(len(r.images)),
	}
	var descriptorPool vk.DescriptorPool
	if res := vk.CreateDescriptorPool(r.device, &poolInfo, nil, &descriptorPool); res != vk.Success {
		return errors.New("unable to create descriptor pool")
	}
	r.descriptorPool = descriptorPool
	return nil
}

func (r *RenderSystem) createDescriptorSets() error {
	r.descriptorSets = make([]vk.DescriptorSet, len(r.images))
	for i := range r.images {
		var set vk.DescriptorSet
		if ret := vk.AllocateDescriptorSets(r.device, &vk.DescriptorSetAllocateInfo{
			SType:              vk.StructureTypeDescriptorSetAllocateInfo,
			DescriptorPool:     r.descriptorPool,
			DescriptorSetCount: 1,
			PSetLayouts:        r.descriptorSetLayouts,
		}, &set); ret != vk.Success {
			return errors.New("Unable to allocate descriptor set")
		}
		r.descriptorSets[i] = set
		bufferInfo := vk.DescriptorBufferInfo{
			Buffer: r.uniformBuffers[i],
			Offset: 0,
			Range:  vk.DeviceSize(int(unsafe.Sizeof(UniformBufferObject{}))),
		}
		imageInfos := make([]vk.DescriptorImageInfo, 0)
		for _, tex := range theTextureLoader.images {
			imageInfos = append(imageInfos, vk.DescriptorImageInfo{
				ImageLayout: vk.ImageLayoutShaderReadOnlyOptimal,
				ImageView:   tex.Texture.view,
				Sampler:     tex.Texture.sampler,
			})
		}
		descriptorWrite := vk.WriteDescriptorSet{
			SType:           vk.StructureTypeWriteDescriptorSet,
			DstSet:          r.descriptorSets[i],
			DstBinding:      0,
			DstArrayElement: 0,
			DescriptorType:  vk.DescriptorTypeUniformBuffer,
			DescriptorCount: 1,
			PBufferInfo:     []vk.DescriptorBufferInfo{bufferInfo},
		}
		imgSamplerWrite := vk.WriteDescriptorSet{
			SType:           vk.StructureTypeWriteDescriptorSet,
			DstSet:          r.descriptorSets[i],
			DstBinding:      1,
			DstArrayElement: 0,
			DescriptorType:  vk.DescriptorTypeCombinedImageSampler,
			DescriptorCount: uint32(len(imageInfos)),
			PImageInfo:      imageInfos,
		}
		writeDescriptorSets := []vk.WriteDescriptorSet{descriptorWrite, imgSamplerWrite}
		vk.UpdateDescriptorSets(r.device, uint32(len(writeDescriptorSets)), writeDescriptorSets, 0, nil)
	}
	return nil
}
