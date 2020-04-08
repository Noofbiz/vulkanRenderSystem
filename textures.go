package vulkanRenderSystem

import (
	"errors"
	"image"
	"unsafe"
	// imported to decode jpegs and upload them to the GPU.
	"image/draw"
	_ "image/jpeg"
	// imported to decode .pngs and upload them to the GPU.
	_ "image/png"
	// imported to decode .gifs and uppload them to the GPU.
	_ "image/gif"
	"io"

	"github.com/EngoEngine/engo"

	vk "github.com/vulkan-go/vulkan"
)

type Texture struct {
	sampler vk.Sampler

	image       vk.Image
	imageLayout vk.ImageLayout

	mem  vk.DeviceMemory
	view vk.ImageView

	texWidth  int32
	texHeight int32
}

func (t *Texture) Destroy(dev vk.Device) {
	vk.DestroySampler(dev, t.sampler, nil)
	vk.DestroyImageView(dev, t.view, nil)
	vk.DestroyImage(dev, t.image, nil)
	vk.FreeMemory(dev, t.mem, nil)
}

func (t *Texture) DestroyImage(dev vk.Device) {
	vk.FreeMemory(dev, t.mem, nil)
	vk.DestroyImage(dev, t.image, nil)
}

func (t *Texture) Width() float32 {
	return float32(t.texWidth)
}

func (t *Texture) Height() float32 {
	return float32(t.texHeight)
}

type TextureResource struct {
	Texture *Texture
	url     string
}

func NewTextureResource(img image.Image, url string) TextureResource {
	if theRenderSystem == nil {
		panic("tried to create NewTextureResource without a vulkan render system setup.")
	}

	tex := &Texture{}

	bounds := img.Bounds()
	nrgba := image.NewNRGBA(bounds)
	draw.Draw(nrgba, bounds, img, image.ZP, draw.Src)
	imgSize := vk.DeviceSize(4 * bounds.Dx() * bounds.Dy())
	stagingBuffer, stagingBufferMemory, err := theRenderSystem.createBuffer(imgSize, vk.BufferUsageFlags(vk.BufferUsageTransferSrcBit), vk.MemoryPropertyFlags(vk.MemoryPropertyHostVisibleBit|vk.MemoryPropertyHostCoherentBit))
	if err != nil {
		panic("[VULKAN RENDER SYSTEM] unable to create staging buffer for image with url: " + url)
	}

	var data unsafe.Pointer
	vk.MapMemory(theRenderSystem.device, stagingBufferMemory, 0, imgSize, 0, &data)
	vk.Memcopy(data, []byte(nrgba.Pix))
	vk.UnmapMemory(theRenderSystem.device, stagingBufferMemory)

	imageInfo := vk.ImageCreateInfo{
		SType:     vk.StructureTypeImageCreateInfo,
		ImageType: vk.ImageType2d,
		Extent: vk.Extent3D{
			Width:  uint32(bounds.Dx()),
			Height: uint32(bounds.Dy()),
			Depth:  1,
		},
		MipLevels:     1,
		ArrayLayers:   1,
		Format:        vk.FormatR8g8b8a8Srgb,
		Tiling:        vk.ImageTilingOptimal,
		InitialLayout: vk.ImageLayoutUndefined,
		Usage:         vk.ImageUsageFlags(vk.ImageUsageTransferDstBit | vk.ImageUsageSampledBit),
		SharingMode:   vk.SharingModeExclusive,
		Samples:       vk.SampleCount1Bit,
	}

	var texImg vk.Image
	if vk.CreateImage(theRenderSystem.device, &imageInfo, nil, &texImg) != vk.Success {
		panic("[VULKAN RENDER SYSTEM] unable to create image from url: " + url)
	}
	tex.image = texImg

	var memRequirements vk.MemoryRequirements
	vk.GetImageMemoryRequirements(theRenderSystem.device, tex.image, &memRequirements)
	memRequirements.Deref()

	memtype, err := theRenderSystem.findMemoryType(memRequirements.MemoryTypeBits, vk.MemoryPropertyFlags(vk.MemoryPropertyDeviceLocalBit))
	if err != nil {
		panic("[VULKAN RENDER SYSTEM] unable to get memory type for image with url: " + url)
	}

	allocInfo := vk.MemoryAllocateInfo{
		SType:           vk.StructureTypeMemoryAllocateInfo,
		AllocationSize:  memRequirements.Size,
		MemoryTypeIndex: memtype,
	}

	var texImMem vk.DeviceMemory
	if vk.AllocateMemory(theRenderSystem.device, &allocInfo, nil, &texImMem) != vk.Success {
		panic("[VULKAN RENDER SYSTEM] unable to allocate image memory for image with url: " + url)
	}

	if vk.BindImageMemory(theRenderSystem.device, tex.image, texImMem, 0) != vk.Success {
		panic("[VULKAN RENDER SYSTEM] unable to bind image memory for image with url: " + url)
	}
	tex.mem = texImMem

	err = theRenderSystem.transitionImageLayout(tex.image, vk.FormatR8g8b8a8Srgb, vk.ImageLayoutUndefined, vk.ImageLayoutTransferDstOptimal)
	if err != nil {
		panic("[VULKAN RENDER SYSTEM] unable to do first layout transition for image with url: " + url + "\n The error was: " + err.Error())
	}
	err = theRenderSystem.copyBufferToImage(stagingBuffer, tex.image, uint32(bounds.Dx()), uint32(bounds.Dy()))
	if err != nil {
		panic("[VULKAN RENDER SYSTEM] unable to copy buffer to image for image with url: " + url + "\n The error was: " + err.Error())
	}
	err = theRenderSystem.transitionImageLayout(tex.image, vk.FormatR8g8b8a8Srgb, vk.ImageLayoutTransferDstOptimal, vk.ImageLayoutShaderReadOnlyOptimal)
	if err != nil {
		panic("[VULKAN RENDER SYSTEM] unable to do the second layout transition for image with url: " + url + "\n The error was: " + err.Error())
	}

	vk.DestroyBuffer(theRenderSystem.device, stagingBuffer, nil)
	vk.FreeMemory(theRenderSystem.device, stagingBufferMemory, nil)

	viewInfo := vk.ImageViewCreateInfo{
		SType:    vk.StructureTypeImageViewCreateInfo,
		Image:    tex.image,
		ViewType: vk.ImageViewType2d,
		Format:   vk.FormatR8g8b8a8Srgb,
		SubresourceRange: vk.ImageSubresourceRange{
			AspectMask:     vk.ImageAspectFlags(vk.ImageAspectColorBit),
			BaseMipLevel:   0,
			LevelCount:     1,
			BaseArrayLayer: 0,
			LayerCount:     1,
		},
	}

	var view vk.ImageView
	if ok := vk.CreateImageView(theRenderSystem.device, &viewInfo, nil, &view); ok != vk.Success {
		panic("[VULKAN RENDER SYSTEM] failed to create texture image view for url: " + url)
	}
	tex.view = view

	samplerInfo := vk.SamplerCreateInfo{
		SType:                   vk.StructureTypeSamplerCreateInfo,
		MagFilter:               vk.FilterLinear,
		MinFilter:               vk.FilterLinear,
		AddressModeU:            vk.SamplerAddressModeRepeat,
		AddressModeV:            vk.SamplerAddressModeRepeat,
		AddressModeW:            vk.SamplerAddressModeRepeat,
		AnisotropyEnable:        vk.Bool32(vk.True),
		MaxAnisotropy:           16,
		BorderColor:             vk.BorderColorIntOpaqueBlack,
		UnnormalizedCoordinates: vk.Bool32(vk.False),
		CompareEnable:           vk.Bool32(vk.False),
		CompareOp:               vk.CompareOpAlways,
		MipmapMode:              vk.SamplerMipmapModeLinear,
		MipLodBias:              0,
		MinLod:                  0,
		MaxLod:                  0,
	}

	var sampler vk.Sampler
	if ok := vk.CreateSampler(theRenderSystem.device, &samplerInfo, nil, &sampler); ok != vk.Success {
		panic("[VULKAN RENDER SYSTEM] failed to create texture sampler for url: " + url)
	}
	tex.sampler = sampler

	return TextureResource{tex, url}
}

func (t TextureResource) URL() string {
	return t.url
}

type textureLoader struct {
	images map[string]TextureResource
}

var theTextureLoader textureLoader

var imagesToAdd []string

func (t *textureLoader) Load(url string, data io.Reader) error {
	if theRenderSystem == nil {
		imagesToAdd = append(imagesToAdd, url)
		return nil
	}
	var img image.Image
	var err error
	if getExt(url) == ".svg" {
		return errors.New("svg support not implemented yet")
	}
	img, _, err = image.Decode(data)
	if err != nil {
		return err
	}
	t.images[url] = NewTextureResource(img, url)
	return nil
}

func (t *textureLoader) Unload(url string) error {
	texRes := t.images[url]
	texRes.Texture.Destroy(theRenderSystem.device)
	delete(t.images, url)
	return nil
}

func (t *textureLoader) Resource(url string) (engo.Resource, error) {
	if res, ok := t.images[url]; ok {
		return res, nil
	}
	return TextureResource{}, errors.New("unable to locate resource with url: " + url)
}

func init() {
	theTextureLoader = textureLoader{images: make(map[string]TextureResource)}
	engo.Files.Register(".jpg", &theTextureLoader)
	engo.Files.Register(".png", &theTextureLoader)
	engo.Files.Register(".gif", &theTextureLoader)
}
