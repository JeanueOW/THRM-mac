//go:build darwin

package temperature

import (
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/TIANLI0/THRM/internal/types"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/sensors"
	"golang.org/x/sys/unix"
)

// macOS has no public CPU/GPU temperature API. gopsutil implements both
// supported mechanisms:
//
//   - Intel Macs: AppleSMC keys such as TC0D/TC0P/TG0D.
//   - Apple Silicon: the IOHIDEventSystemClient PMU temperature sensors.
//
// Both are in-process reads, so the generic telemetry throttle is unnecessary.
// A short cache avoids reading SMC twice per sample because the generic path
// asks for a cheap CPU value in addition to the platform telemetry.

const (
	darwinSensorCacheTTL = 400 * time.Millisecond
	darwinSensorMaxStale = 10 * time.Second
)

type darwinSensorSnapshot struct {
	cpuTemp         int
	gpuTemp         int
	cpuPower        float64
	gpuPower        float64
	cpuSensors      []types.TemperatureSensor
	gpuSensors      []types.TemperatureSensor
	cpuPowerSensors []types.PowerSensor
	gpuPowerSensors []types.PowerSensor
	gpuModel        string
	gpuVendor       string
	apple           bool
}

var darwinSensorState struct {
	sync.Mutex
	at    time.Time
	snap  darwinSensorSnapshot
	valid bool
}

func darwinIsAppleSilicon() bool {
	if runtime.GOARCH == "arm64" {
		return true
	}
	if value, err := unix.SysctlUint32("hw.optional.arm64"); err == nil && value == 1 {
		return true
	}
	return false
}

func readDarwinSensors() darwinSensorSnapshot {
	now := time.Now()

	darwinSensorState.Lock()
	if darwinSensorState.valid && now.Sub(darwinSensorState.at) < darwinSensorCacheTTL {
		snap := darwinSensorState.snap
		darwinSensorState.Unlock()
		return snap
	}
	darwinSensorState.Unlock()

	snap, ok := sampleDarwinSensors()
	if !ok {
		// A single failed IOKit read must not look like a cooler emergency.
		// Reuse a recent good sample; the caller ignores stale data after
		// darwinSensorMaxStale anyway.
		darwinSensorState.Lock()
		if darwinSensorState.valid && now.Sub(darwinSensorState.at) < darwinSensorMaxStale {
			snap = darwinSensorState.snap
			darwinSensorState.Unlock()
			return snap
		}
		darwinSensorState.Unlock()
		return darwinSensorSnapshot{apple: darwinIsAppleSilicon()}
	}

	darwinSensorState.Lock()
	darwinSensorState.snap = snap
	darwinSensorState.at = now
	darwinSensorState.valid = true
	darwinSensorState.Unlock()

	return snap
}

func sampleDarwinSensors() (darwinSensorSnapshot, bool) {
	apple := darwinIsAppleSilicon()
	snap := darwinSensorSnapshot{apple: apple}

	var smcGPUPower float64
	var smcGPUPowerSensors []types.PowerSensor

	if apple {
		// Apple Silicon exposes temperatures through IOHID; AppleSMC only
		// provides power rails on some models, so use a separate short session.
		if stats, err := sensors.SensorsTemperatures(); err == nil {
			sampleDarwinTemperatures(&snap, stats, true)
		}
		cpuPower, cpuPowerSensors, gpuPower, gpuPowerSensors := readDarwinSMCPower()
		snap.cpuPower = cpuPower
		snap.cpuPowerSensors = cpuPowerSensors
		smcGPUPower = gpuPower
		smcGPUPowerSensors = gpuPowerSensors
	} else {
		// Intel: read temperatures and power from one AppleSMC session, so
		// gopsutil's own SMC connection does not invalidate ours.
		smc := readDarwinSMCAll()
		snap.cpuTemp = smc.cpuTemp
		snap.gpuTemp = smc.gpuTemp
		snap.cpuSensors = smc.cpuSensors
		snap.gpuSensors = smc.gpuSensors
		snap.cpuPower = smc.cpuPower
		snap.cpuPowerSensors = smc.cpuPowerSensors
		smcGPUPower = smc.gpuPower
		smcGPUPowerSensors = smc.gpuPowerSensors

		// Some Intel machines use different SMC keys; fall back to gopsutil
		// (which has a broader key list) if our known keys all failed.
		if snap.cpuTemp == 0 && snap.gpuTemp == 0 {
			if stats, err := sensors.SensorsTemperatures(); err == nil {
				sampleDarwinTemperatures(&snap, stats, false)
			}
		}
	}

	gpuInfo := readDarwinGPUInfo()
	if gpuInfo.model != "" {
		snap.gpuModel = gpuInfo.model
		snap.gpuVendor = gpuVendorFromModel(gpuInfo.model)
	}

	switch {
	case gpuInfo.powerW > 0:
		snap.gpuPower = gpuInfo.powerW
		snap.gpuPowerSensors = darwinGPUInfoPowerSensors(snap.gpuPower)
	case smcGPUPower > 0:
		snap.gpuPower = smcGPUPower
		snap.gpuPowerSensors = smcGPUPowerSensors
	}

	if snap.gpuTemp == 0 && gpuInfo.tempC > 0 {
		snap.gpuTemp = gpuInfo.tempC
		snap.gpuSensors = append(snap.gpuSensors, types.TemperatureSensor{
			Key:   "perf.temp",
			Name:  "GPU Driver Temperature",
			Value: gpuInfo.tempC,
		})
	}
	if snap.gpuModel == "" {
		if apple {
			// The IOAccelerator node does not expose a model on every Apple
			// Silicon generation; the CPU brand string ("Apple M1 Pro") is the
			// best available name for the integrated GPU.
			if cpuModel := darwinCPUInfoModel(); strings.HasPrefix(cpuModel, "Apple") {
				snap.gpuModel = cpuModel
			} else {
				snap.gpuModel = "Apple GPU"
			}
			snap.gpuVendor = "apple"
		} else {
			snap.gpuModel = "GPU"
			snap.gpuVendor = "unknown"
		}
	}

	return snap, snap.cpuTemp > 0 || snap.gpuTemp > 0 || snap.cpuPower > 0 || snap.gpuPower > 0 || snap.gpuModel != "GPU"
}

func sampleDarwinTemperatures(snap *darwinSensorSnapshot, stats []sensors.TemperatureStat, apple bool) {
	seen := make(map[string]struct{}, len(stats))

	for _, stat := range stats {
		key := strings.TrimSpace(stat.SensorKey)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		value := int(math.Round(stat.Temperature))
		if value < 1 || value > 125 {
			continue
		}

		if apple {
			lower := strings.ToLower(key)
			switch {
			case strings.Contains(lower, "tdie") || strings.Contains(lower, "cpu") || strings.Contains(lower, "core"):
				snap.cpuSensors = append(snap.cpuSensors, darwinSensor(key, value))
			case strings.Contains(lower, "gpu") || strings.Contains(lower, "tgp"):
				snap.gpuSensors = append(snap.gpuSensors, darwinSensor(key, value))
			default:
				continue
			}
		} else {
			upper := strings.ToUpper(key)
			switch {
			case upper == "TCGC" || strings.HasPrefix(upper, "TG"):
				snap.gpuSensors = append(snap.gpuSensors, darwinSensor(upper, value))
			case strings.HasPrefix(upper, "TC0"):
				snap.cpuSensors = append(snap.cpuSensors, darwinSensor(upper, value))
			default:
				continue
			}
		}
		seen[key] = struct{}{}
	}

	snap.cpuTemp = averageSensorValue(snap.cpuSensors)
	snap.gpuTemp = averageSensorValue(snap.gpuSensors)
}

func darwinSensor(key string, value int) types.TemperatureSensor {
	names := map[string]string{
		"TC0D": "CPU Diode",
		"TC0E": "CPU Diode (filtered)",
		"TC0F": "CPU Diode (adjusted)",
		"TC0P": "CPU Proximity",
		"TC0H": "CPU Heatsink",
		"TG0D": "GPU Diode",
		"TG0P": "GPU Proximity",
		"TG0H": "GPU Heatsink",
		"TGDD": "GPU AMD Diode",
		"TCGC": "GPU Intel Graphics",
	}
	name, ok := names[key]
	if !ok {
		name = key
	}
	return types.TemperatureSensor{Key: key, Name: name, Value: value}
}

func averageSensorValue(list []types.TemperatureSensor) int {
	if len(list) == 0 {
		return 0
	}
	sum, n := 0, 0
	for _, sensor := range list {
		if sensor.Value <= 0 {
			continue
		}
		sum += sensor.Value
		n++
	}
	if n == 0 {
		return 0
	}
	return (sum + n/2) / n
}

var (
	darwinCPUModelOnce sync.Once
	darwinCPUModel     string
)

func darwinCPUInfoModel() string {
	darwinCPUModelOnce.Do(func() {
		if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
			darwinCPUModel = strings.TrimSpace(infos[0].ModelName)
		}
	})
	return darwinCPUModel
}

// readSensorCPUTemp satisfies the generic "cheap CPU temperature" hook.
func (r *Reader) readSensorCPUTemp() int {
	return readDarwinSensors().cpuTemp
}
