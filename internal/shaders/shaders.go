package shaders

//go:generate glslangvalidator -V shader.frag shader.vert
//go:generate go-bindata -nocompress -pkg=shaders frag.spv vert.spv
//go:generate gofmt -s -w .
