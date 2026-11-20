//go:build darwin

package temperature

import "github.com/TIANLI0/THRM/internal/types"

// platformTelemetryIsExpensive 表示平台遥测是否需要创建外部进程。
const platformTelemetryIsExpensive = false

func readPlatformTelemetry(r *Reader, disableGPU bool) telemetryReading {
	snap := readDarwinSensors()

	reading := telemetryReading{
		cpuTemp:         snap.cpuTemp,
		cpuPower:        snap.cpuPower,
		cpuSensors:      snap.cpuSensors,
		cpuPowerSensors: snap.cpuPowerSensors,
		cpuModel:        darwinCPUInfoModel(),
	}
	if disableGPU {
		return reading
	}

	reading.gpuTemp = snap.gpuTemp
	reading.gpuPower = snap.gpuPower
	reading.gpuSensors = snap.gpuSensors
	reading.gpuPowerSensors = snap.gpuPowerSensors
	reading.gpuModel = snap.gpuModel
	if len(snap.gpuSensors) > 0 || snap.gpuPower > 0 {
		reading.gpuDevices = []types.TemperatureGPUDevice{{
			Key:          "darwin-gpu",
			Name:         snap.gpuModel,
			Vendor:       snap.gpuVendor,
			Sensors:      snap.gpuSensors,
			PowerSensors: snap.gpuPowerSensors,
		}}
		reading.selectedGpuDevice = "darwin-gpu"
	}
	return reading
}
