#Vulkan Render System

This is a replacement for common's Render system in [engo](https://engoengine.github.io) that uses Vulkan
instead of OpenGL to render to screens.

### Getting Started

First, you'll need the Vulkan SDK on your system.

#### Linux / Windows

On linux and windows make sure you have [LunarG's Vulkan SDK](https://www.lunarg.com/vulkan-sdk/)
installed.

#### OSX

On OSX make sure you have [MoltenVK](https://github.com/KhronosGroup/MoltenVK)
installed.

### Building with engo

Use the build tag vulkan from `engo` to run.

`go run --tags=vulkan *.go`

## TODO

In order to make this a replacement for the common.RenderSystem's stuff, it
has to be able to do the following exactly as the regular RenderSystem does:

[ ] use .png .jpg .bmp and .svg images
[ ] blit an image to the screen
[ ] blit multiple images to the screen at locations based on their space component
[ ] animation
[ ] hud vs non-hud elements
[ ] text from .ttf and .otf
[ ] TMX maps
[ ] Global Scale
[ ] Scale on Resize
[ ] Full Screen
[ ] Utilize custom shaders
[ ] View Culling
[ ] Blend Maps
