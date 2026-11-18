//go:build noasm || (!amd64 && !arm64 && !(riscv64 && riscv64.rva23u64 && go1.26))

package webp

func dspInit() {}
