//go:build darwin

#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <stdlib.h>

#include "gpu_info_darwin.h"

static int thrmDictNumber(CFDictionaryRef dict, CFStringRef key, double *out) {
    CFTypeRef value = CFDictionaryGetValue(dict, key);
    if (value == NULL) {
        return 0;
    }
    if (CFGetTypeID(value) == CFNumberGetTypeID()) {
        double number = 0;
        if (CFNumberGetValue((CFNumberRef)value, kCFNumberDoubleType, &number)) {
            *out = number;
            return 1;
        }
        int64_t integer = 0;
        if (CFNumberGetValue((CFNumberRef)value, kCFNumberSInt64Type, &integer)) {
            *out = (double)integer;
            return 1;
        }
        return 0;
    }
    if (CFGetTypeID(value) == CFStringGetTypeID()) {
        char buffer[64] = {0};
        if (CFStringGetCString((CFStringRef)value, buffer, sizeof(buffer), kCFStringEncodingUTF8)) {
            *out = atof(buffer);
            return 1;
        }
    }
    return 0;
}

static void thrmTrimModel(char *buffer) {
    if (buffer == NULL || buffer[0] == 0) {
        return;
    }
    char *start = buffer;
    while (*start == '"' || *start == ' ' || *start == '\t' || *start == '\n' || *start == '\r') {
        start++;
    }
    size_t length = strlen(start);
    while (length > 0) {
        char tail = start[length - 1];
        if (tail != '"' && tail != ' ' && tail != '\t' && tail != '\n' && tail != '\r') {
            break;
        }
        start[--length] = 0;
    }
    if (start != buffer) {
        memmove(buffer, start, length + 1);
    }
}

static int thrmCopyStringValue(CFTypeRef value, char *buffer, size_t bufferLen) {
    if (value == NULL || bufferLen == 0) {
        return 0;
    }
    if (CFGetTypeID(value) == CFStringGetTypeID()) {
        if (CFStringGetCString((CFStringRef)value, buffer, bufferLen, kCFStringEncodingUTF8)) {
            thrmTrimModel(buffer);
            return buffer[0] != 0;
        }
        return 0;
    }
    if (CFGetTypeID(value) == CFDataGetTypeID()) {
        CFDataRef data = (CFDataRef)value;
        CFIndex length = CFDataGetLength(data);
        const UInt8 *bytes = CFDataGetBytePtr(data);
        if (length <= 0 || bytes == NULL) {
            return 0;
        }
        if (length >= (CFIndex)bufferLen) {
            length = (CFIndex)bufferLen - 1;
        }
        memcpy(buffer, bytes, (size_t)length);
        buffer[length] = 0;
        thrmTrimModel(buffer);
        return buffer[0] != 0;
    }
    return 0;
}

static int thrmIsDiscreteModel(const char *model) {
    if (model == NULL) {
        return 0;
    }
    char lower[256];
    size_t i = 0;
    for (; model[i] != 0 && i + 1 < sizeof(lower); i++) {
        char c = model[i];
        lower[i] = (c >= 'A' && c <= 'Z') ? (char)(c - 'A' + 'a') : c;
    }
    lower[i] = 0;
    return strstr(lower, "nvidia") != NULL || strstr(lower, "geforce") != NULL ||
           strstr(lower, "amd") != NULL || strstr(lower, "radeon") != NULL;
}

static int thrmModelFromEntry(io_registry_entry_t entry, char *buffer, size_t bufferLen) {
    if (entry == 0) {
        return 0;
    }
    CFTypeRef model = IORegistryEntryCreateCFProperty(entry, CFSTR("model"), kCFAllocatorDefault, 0);
    if (model == NULL) {
        model = IORegistryEntryCreateCFProperty(entry, CFSTR("IOName"), kCFAllocatorDefault, 0);
    }
    if (model == NULL) {
        return 0;
    }
    int ok = thrmCopyStringValue(model, buffer, bufferLen);
    CFRelease(model);
    return ok;
}

static int thrmVendorFromEntry(io_registry_entry_t entry, char *buffer, size_t bufferLen) {
    if (entry == 0 || bufferLen == 0) {
        return 0;
    }
    CFTypeRef vendor = IORegistryEntryCreateCFProperty(entry, CFSTR("vendor-id"), kCFAllocatorDefault, 0);
    if (vendor == NULL || CFGetTypeID(vendor) != CFDataGetTypeID()) {
        if (vendor != NULL) {
            CFRelease(vendor);
        }
        return 0;
    }
    CFDataRef data = (CFDataRef)vendor;
    if (CFDataGetLength(data) < 2) {
        CFRelease(vendor);
        return 0;
    }
    const UInt8 *bytes = CFDataGetBytePtr(data);
    uint16_t vendorID = (uint16_t)(bytes[0] | (bytes[1] << 8));
    CFRelease(vendor);
    switch (vendorID) {
    case 0x10de:
        snprintf(buffer, bufferLen, "NVIDIA GPU");
        return 1;
    case 0x1002:
        snprintf(buffer, bufferLen, "AMD GPU");
        return 1;
    case 0x8086:
        snprintf(buffer, bufferLen, "Intel GPU");
        return 1;
    default:
        return 0;
    }
}

// thrmReadGPUInfo walks the IOAccelerator services and picks the accelerator
// with the highest reported power (falling back to the first one with a model).
// PerformanceStatistics is the same dictionary iStat Menus reads; NVIDIA,
// AMD and Intel drivers publish different subsets of the keys below.
int thrmReadGPUInfo(thrm_gpu_info_t *out) {
    if (out == NULL) {
        return -1;
    }
    memset(out, 0, sizeof(*out));

    io_iterator_t iterator = 0;
    if (IOServiceGetMatchingServices(MACH_PORT_NULL, IOServiceMatching("IOAccelerator"), &iterator) != KERN_SUCCESS) {
        return -1;
    }

    double bestPower = 0;
    int bestPowerOK = 0;
    int bestTemp = 0;
    char bestModel[256] = {0};
    int bestModelOK = 0;
    char anyModel[256] = {0};
    int anyModelOK = 0;
    char discreteModel[256] = {0};
    int discreteModelOK = 0;
    int any = 0;

    io_object_t service = 0;
    while ((service = IOIteratorNext(iterator)) != 0) {
        any = 1;
        double power = 0;
        int powerOK = 0;

        CFTypeRef stats = IORegistryEntryCreateCFProperty(service, CFSTR("PerformanceStatistics"), kCFAllocatorDefault, 0);
        if (stats != NULL && CFGetTypeID(stats) == CFDictionaryGetTypeID()) {
            CFDictionaryRef dict = (CFDictionaryRef)stats;
            if (thrmDictNumber(dict, CFSTR("Total Power(W)"), &power) && power > 0) {
                powerOK = 1;
            } else if (thrmDictNumber(dict, CFSTR("GPU Power(W)"), &power) && power > 0) {
                powerOK = 1;
            } else if (thrmDictNumber(dict, CFSTR("Instant Power(W)"), &power) && power > 0) {
                powerOK = 1;
            } else if (thrmDictNumber(dict, CFSTR("RUSD Power(mW)"), &power) && power > 0) {
                power /= 1000.0;
                powerOK = 1;
            }
            double temp = 0;
            if (thrmDictNumber(dict, CFSTR("GPU Temperature(C)"), &temp) && temp > 0) {
                bestTemp = (int)(temp + 0.5);
            } else if (thrmDictNumber(dict, CFSTR("Temperature(C)"), &temp) && temp > 0) {
                bestTemp = (int)(temp + 0.5);
            }
        }
        if (stats != NULL) {
            CFRelease(stats);
        }

        char model[256] = {0};
        int modelOK = thrmModelFromEntry(service, model, sizeof(model));
        io_registry_entry_t current = service;
        IOObjectRetain(current);
        for (int depth = 0; depth < 4 && !modelOK; depth++) {
            io_registry_entry_t parent = 0;
            if (IORegistryEntryGetParentEntry(current, kIOServicePlane, &parent) != KERN_SUCCESS) {
                parent = 0;
            }
            IOObjectRelease(current);
            current = parent;
            if (current == 0) {
                break;
            }
            modelOK = thrmModelFromEntry(current, model, sizeof(model));
            if (!modelOK) {
                modelOK = thrmVendorFromEntry(current, model, sizeof(model));
            }
        }
        if (current != 0) {
            IOObjectRelease(current);
        }

        if (powerOK && (!bestPowerOK || power > bestPower)) {
            bestPower = power;
            bestPowerOK = 1;
            if (modelOK) {
                memcpy(bestModel, model, sizeof(bestModel));
                bestModelOK = 1;
            }
        }
        if (modelOK) {
            if (!anyModelOK) {
                memcpy(anyModel, model, sizeof(anyModel));
                anyModelOK = 1;
            }
            if (thrmIsDiscreteModel(model)) {
                memcpy(discreteModel, model, sizeof(discreteModel));
                discreteModelOK = 1;
            }
        }
        IOObjectRelease(service);
    }
    IOObjectRelease(iterator);

    if (!any) {
        return -1;
    }
    if (bestPowerOK) {
        out->powerW = bestPower;
        out->powerOK = 1;
    }
    if (bestTemp > 0) {
        out->tempC = bestTemp;
        out->tempOK = 1;
    }
    if (bestModelOK) {
        memcpy(out->model, bestModel, sizeof(out->model));
        out->modelOK = 1;
    } else if (discreteModelOK) {
        memcpy(out->model, discreteModel, sizeof(out->model));
        out->modelOK = 1;
    } else if (anyModelOK) {
        memcpy(out->model, anyModel, sizeof(out->model));
        out->modelOK = 1;
    }
    return 0;
}

const char *thrmGPUInfoModel(thrm_gpu_info_t *info) {
    return (info != NULL) ? info->model : "";
}
