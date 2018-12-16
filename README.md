#Vulkan Render System

This is a replacement for common's Render system in `engo` that uses Vulkan
instead of OpenGL to render to screens.

### Getting Started

First, you'll need the Vulkan SDK on your system.

#### Linux / Windows

On linux and windows make sure you have [LunarG's Vulkan SDK](https://www.lunarg.com/vulkan-sdk/)
installed.

#### OSX

On OSX make sure you have [MoltenVK](https://github.com/KhronosGroup/MoltenVK)
installed.

Currently this only works with SDL. Use the build tag sdl to run.
`go run --tags=sdl *.go`
