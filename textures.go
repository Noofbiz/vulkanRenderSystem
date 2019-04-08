package vulkanRenderSystem

import (
	"io"

	"github.com/EngoEngine/engo"
)

type textureLoader struct{}

func (t *textureLoader) Load(url string, data io.Reader) error {
	return nil
}

func (t *textureLoader) Unload(url string) error {
	return nil
}

func (t *textureLoader) Resource(url string) (engo.Resource, error) {
	return nil, nil
}
