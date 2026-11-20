//go:build darwin

package temperature

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include "gpu_info_darwin.h"
*/
import "C"

import (
	"strings"

	"github.com/TIANLI0/THRM/internal/types"
)

type darwinGPUInfo struct {
	powerW float64
	tempC  int
	model  string
}

// readDarwinGPUInfo reads the accelerator's PerformanceStatistics dictionary:
// power, the driver-reported temperature, and the PCI model name. The same
// dictionary is used by iStat Menus and is populated by NVIDIA, AMD and Intel
// drivers, so it works without any third-party kext.
func readDarwinGPUInfo() darwinGPUInfo {
	var info C.thrm_gpu_info_t
	if C.thrmReadGPUInfo(&info) != 0 {
		return darwinGPUInfo{}
	}
	result := darwinGPUInfo{
		tempC: int(info.tempC),
	}
	if info.powerOK != 0 {
		result.powerW = roundPower(float64(info.powerW))
	}
	if info.modelOK != 0 {
		result.model = strings.TrimSpace(C.GoString(C.thrmGPUInfoModel(&info)))
	}
	return result
}

func darwinGPUInfoPowerSensors(power float64) []types.PowerSensor {
	if power <= 0 {
		return nil
	}
	return []types.PowerSensor{{Key: "gpu.total", Name: "GPU Total Power", Value: power}}
}

func gpuVendorFromModel(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "nvidia"), strings.Contains(lower, "geforce"):
		return "nvidia"
	case strings.Contains(lower, "amd"), strings.Contains(lower, "radeon"):
		return "amd"
	case strings.Contains(lower, "intel"):
		return "intel"
	case strings.Contains(lower, "apple"):
		return "apple"
	default:
		return "unknown"
	}
}
