//+build demo

package main

import (
	"github.com/EngoEngine/ecs"
	"github.com/EngoEngine/engo"

	"github.com/Noofbiz/vulkanRenderSystem"
)

type DefaultScene struct {
	renderSystem vulkanRenderSystem.RenderSystem
}

type Guy struct {
	ecs.BasicEntity
}

func (d *DefaultScene) Preload() {
	engo.Files.Load("texture.jpg")
}

func (d *DefaultScene) Setup(u engo.Updater) {
	w, _ := u.(*ecs.World)
	w.AddSystem(&d.renderSystem)
}

func (*DefaultScene) Type() string { return "GameWorld" }

func (d *DefaultScene) Exit() {
	d.renderSystem.Cleanup()
}

func main() {
	opts := engo.RunOptions{
		Title:                      "Hello World Demo",
		Width:                      1024,
		Height:                     640,
		ApplicationMajorVersion:    1,
		ApplicationMinorVersion:    0,
		ApplicationRevisionVersion: 0,
	}
	engo.Run(opts, &DefaultScene{})
}
