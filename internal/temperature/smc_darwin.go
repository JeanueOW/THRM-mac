//go:build darwin

package temperature

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdint.h>

int thrmSMCConnect(void);
void thrmSMCClose(void);
int thrmSMCRead(uint32_t key, void *outBytes, int *outSize, uint32_t *outType);
*/
import "C"

import (
	"encoding/binary"
	"math"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/TIANLI0/THRM/internal/types"
)

// A single AppleSMC user client is opened per sample and shared by the
// temperature and power reads. gopsutil's Intel temperature reader also opens
// AppleSMC; on VirtualSMC a connection that stays open while another client
// opens and closes its handle stops returning data, so keeping our connection
// and gopsutil's apart is not reliable. One session per sample avoids that and
// costs only a few IOKit calls.
var darwinSMCMutex sync.Mutex

type darwinSMCSession struct {
	open bool
}

func openDarwinSMCSession() *darwinSMCSession {
	darwinSMCMutex.Lock()
	if C.thrmSMCConnect() != 0 {
		darwinSMCMutex.Unlock()
		return nil
	}
	return &darwinSMCSession{open: true}
}

func (s *darwinSMCSession) Close() {
	if s == nil || !s.open {
		return
	}
	C.thrmSMCClose()
	s.open = false
	darwinSMCMutex.Unlock()
}

// Power keys are tried in order. PCTR/PCPT report total package power (cores +
// uncore); PCPC/PC0C report the CPU core rail. Windows' PawnIO reading is a
// package value, so the total is the closest match.
var (
	darwinSMCCPUPowerKeys = []string{"PCTR", "PCPT", "PCPC", "PCAM", "PC0C", "PC0R"}
	darwinSMCGPUPowerKeys = []string{"PG0C", "PG0R", "PGDD", "PGRC", "PCGC"}
)

type darwinSMCTemperatureKey struct {
	key  string
	name string
}

var (
	darwinSMCCPUTemperatureKeys = []darwinSMCTemperatureKey{
		{key: "TC0D", name: "CPU Diode"},
		{key: "TC0E", name: "CPU Diode (filtered)"},
		{key: "TC0F", name: "CPU Diode (adjusted)"},
		{key: "TC0P", name: "CPU Proximity"},
		{key: "TC0H", name: "CPU Heatsink"},
	}
	darwinSMCGPUTemperatureKeys = []darwinSMCTemperatureKey{
		{key: "TG0D", name: "GPU Diode"},
		{key: "TG0P", name: "GPU Proximity"},
		{key: "TG0H", name: "GPU Heatsink"},
		{key: "TGDD", name: "GPU AMD Diode"},
		{key: "TCGC", name: "GPU Intel Graphics"},
	}
)

func (s *darwinSMCSession) Read(key string) (float64, bool) {
	if s == nil || !s.open || len(key) != 4 {
		return 0, false
	}
	var raw [32]byte
	var size C.int
	var dataType C.uint32_t
	status := C.thrmSMCRead(C.uint32_t(darwinSMCFourCC(key)), unsafe.Pointer(&raw[0]), &size, &dataType)
	if status != 0 || size <= 0 || size > 32 {
		return 0, false
	}
	return decodeDarwinSMCValue(darwinSMCTypeString(uint32(dataType)), int(size), raw[:int(size)])
}

func (s *darwinSMCSession) readPower(keys []string) (float64, string, bool) {
	for attempt := 0; attempt < 2; attempt++ {
		for _, key := range keys {
			value, ok := s.Read(key)
			if !ok || value <= 0 || value > 1000 {
				continue
			}
			return roundPower(value), key, true
		}
		// VirtualSMC occasionally NAKs a burst of reads while another client
		// is polling. One short retry removes most zero-power samples.
		if attempt == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	return 0, "", false
}

func (s *darwinSMCSession) readTemperatures(keys []darwinSMCTemperatureKey) []types.TemperatureSensor {
	sensors := make([]types.TemperatureSensor, 0, len(keys))
	for _, entry := range keys {
		value, ok := s.Read(entry.key)
		if !ok {
			continue
		}
		rounded := int(math.Round(value))
		if rounded < 1 || rounded > 125 {
			continue
		}
		sensors = append(sensors, types.TemperatureSensor{Key: entry.key, Name: entry.name, Value: rounded})
	}
	return sensors
}

type darwinSMCReading struct {
	cpuTemp         int
	gpuTemp         int
	cpuSensors      []types.TemperatureSensor
	gpuSensors      []types.TemperatureSensor
	cpuPower        float64
	cpuPowerSensors []types.PowerSensor
	gpuPower        float64
	gpuPowerSensors []types.PowerSensor
}

// readDarwinSMCAll reads temperatures and power from one AppleSMC session.
// Used on Intel Macs, where it replaces gopsutil's SMC temperature reader.
func readDarwinSMCAll() darwinSMCReading {
	reading := darwinSMCReading{}

	// Temperatures first: they drive fan control and must not be lost when the
	// SMC is busy. A second, short session reads the power rails; VirtualSMC
	// occasionally NAKs a long uninterrupted burst of key reads.
	if session := openDarwinSMCSession(); session != nil {
		reading.cpuSensors = session.readTemperatures(darwinSMCCPUTemperatureKeys)
		reading.gpuSensors = session.readTemperatures(darwinSMCGPUTemperatureKeys)
		reading.cpuTemp = averageSensorValue(reading.cpuSensors)
		reading.gpuTemp = averageSensorValue(reading.gpuSensors)
		session.Close()
	}

	if session := openDarwinSMCSession(); session != nil {
		if value, key, ok := session.readPower(darwinSMCCPUPowerKeys); ok {
			reading.cpuPower = value
			reading.cpuPowerSensors = []types.PowerSensor{{Key: "cpu." + strings.ToLower(key), Name: "CPU Package Power", Value: value}}
		}
		if value, key, ok := session.readPower(darwinSMCGPUPowerKeys); ok {
			reading.gpuPower = value
			reading.gpuPowerSensors = []types.PowerSensor{{Key: "gpu." + strings.ToLower(key), Name: "GPU Power", Value: value}}
		}
		session.Close()
	}
	return reading
}

// readDarwinSMCPower is the power-only path used on Apple Silicon, where
// gopsutil uses IOHID rather than AppleSMC for temperatures.
func readDarwinSMCPower() (cpuPower float64, cpuSensors []types.PowerSensor, gpuPower float64, gpuSensors []types.PowerSensor) {
	session := openDarwinSMCSession()
	if session == nil {
		return 0, nil, 0, nil
	}
	defer session.Close()

	if value, key, ok := session.readPower(darwinSMCCPUPowerKeys); ok {
		cpuPower = value
		cpuSensors = []types.PowerSensor{{Key: "cpu." + strings.ToLower(key), Name: "CPU Package Power", Value: value}}
	}
	if value, key, ok := session.readPower(darwinSMCGPUPowerKeys); ok {
		gpuPower = value
		gpuSensors = []types.PowerSensor{{Key: "gpu." + strings.ToLower(key), Name: "GPU Power", Value: value}}
	}
	return cpuPower, cpuSensors, gpuPower, gpuSensors
}

func darwinSMCFourCC(key string) uint32 {
	var value uint32
	for i := 0; i < 4; i++ {
		value = value<<8 | uint32(key[i])
	}
	return value
}

func darwinSMCTypeString(value uint32) string {
	return string([]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}

func decodeDarwinSMCValue(dataType string, size int, data []byte) (float64, bool) {
	if size < 2 {
		if size == 1 && dataType == "ui8 " {
			return float64(data[0]), true
		}
		return 0, false
	}
	unsigned16 := binary.BigEndian.Uint16(data[:2])
	signed16 := int16(unsigned16)
	switch dataType {
	case "sp96": // signed 9.6 fixed point, used by VirtualSMC CPU power keys
		return float64(signed16) / 64, true
	case "sp78":
		return float64(signed16) / 256, true
	case "sp87":
		return float64(signed16) / 128, true
	case "sp69":
		return float64(signed16) / 512, true
	case "sp5a":
		return float64(signed16) / 1024, true
	case "sp4b":
		return float64(signed16) / 2048, true
	case "sp3c":
		return float64(signed16) / 4096, true
	case "spb4":
		return float64(signed16) / 16, true
	case "spa5":
		return float64(signed16) / 2048, true
	case "fpe2":
		return float64(unsigned16) / 4, true
	case "ui16":
		return float64(unsigned16), true
	case "si16":
		return float64(signed16), true
	}
	if size >= 4 {
		switch dataType {
		case "flt ":
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(data[:4]))), true
		case "ui32":
			return float64(binary.BigEndian.Uint32(data[:4])), true
		case "si32":
			return float64(int32(binary.BigEndian.Uint32(data[:4]))), true
		}
	}
	return 0, false
}

func roundPower(value float64) float64 {
	return math.Round(value*10) / 10
}
