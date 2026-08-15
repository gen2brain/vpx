//go:build noasm || (!amd64 && !arm64 && !(riscv64 && riscv64.rva23u64))

package webp

func dspInit() {}
