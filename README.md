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
