package main

import "golang.org/x/sys/cpu"

// fastRVVMSupported reports whether the host CPU exposes the instruction
// sets the optimized RVVM build was compiled against (AVX2/FMA3/AES - see
// docs/phase4-spike-results.md). Deliberately does NOT check for
// BMI1/BMI2: a Round 4 test machine's real hardware had this masked by its
// hypervisor despite the underlying silicon supporting it, so the
// optimized build is intentionally compiled without relying on BMI2 and
// this check matches that.
func fastRVVMSupported() bool {
	return cpu.X86.HasAVX2 && cpu.X86.HasFMA && cpu.X86.HasAES
}
