package vulkanRenderSystem

import (
	"errors"
	"image"
	// imported to decode jpegs and upload them to the GPU.
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

	memAlloc *vk.MemoryAllocateInfo
	mem      vk.DeviceMemory
	view     vk.ImageView

	texWidth  int32
	texHeight int32
}

func (t *Texture) Destroy(dev vk.Device) {
	vk.DestroyImageView(dev, t.view, nil)
	vk.FreeMemory(dev, t.mem, nil)
	vk.DestroyImage(dev, t.image, nil)
	vk.DestroySampler(dev, t.sampler, nil)
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
	return TextureResource{tex, url}
}

func (t *TextureResource) URL() string {
	return t.url
}

type textureLoader struct {
	images map[string]TextureResource
}

var theTextureLoader textureLoader

var imagesToAdd = make(map[string]io.Reader)

func (t *textureLoader) Load(url string, data io.Reader) error {
	if theRenderSystem == nil {
		imagesToAdd[url] = data
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
	return nil
}

func (t *textureLoader) Resource(url string) (engo.Resource, error) {
	return nil, nil
}

func init() {
	theTextureLoader = textureLoader{images: make(map[string]TextureResource)}
	engo.Files.Register(".jpg", &theTextureLoader)
	engo.Files.Register(".png", &theTextureLoader)
	engo.Files.Register(".gif", &theTextureLoader)
}
