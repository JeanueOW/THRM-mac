//go:build darwin

#include <IOKit/IOKitLib.h>
#include <stdint.h>
#include <string.h>

// AppleSMC data structures (same layout used by VirtualSMC, iStat and the
// Intel SMC tools). Power keys such as PCTR/PCPC are two-byte fixed-point
// values; the Go side decodes them based on the returned data type.

typedef struct {
    char major;
    char minor;
    char build;
    char reserved[1];
    uint16_t release;
} thrm_smc_vers_t;

typedef struct {
    uint16_t version;
    uint16_t length;
    uint32_t cpuPLimit;
    uint32_t gpuPLimit;
    uint32_t memPLimit;
} thrm_smc_plimit_t;

typedef struct {
    uint32_t dataSize;
    uint32_t dataType;
    char dataAttributes;
} thrm_smc_key_info_t;

typedef struct {
    uint32_t key;
    thrm_smc_vers_t vers;
    thrm_smc_plimit_t pLimitData;
    thrm_smc_key_info_t keyInfo;
    char result;
    char status;
    char data8;
    uint32_t data32;
    uint8_t bytes[32];
} thrm_smc_key_data_t;

static io_connect_t gSMCConnection = 0;

int thrmSMCConnect(void) {
    if (gSMCConnection != 0) {
        return 0;
    }
    io_service_t service = IOServiceGetMatchingService(MACH_PORT_NULL, IOServiceMatching("AppleSMC"));
    if (service == 0) {
        return -1;
    }
    kern_return_t result = IOServiceOpen(service, mach_task_self(), 0, &gSMCConnection);
    IOObjectRelease(service);
    if (result != kIOReturnSuccess) {
        gSMCConnection = 0;
        return -2;
    }
    return 0;
}

void thrmSMCClose(void) {
    if (gSMCConnection != 0) {
        IOServiceClose(gSMCConnection);
        gSMCConnection = 0;
    }
}

static int thrmSMCCall(thrm_smc_key_data_t *input, thrm_smc_key_data_t *output) {
    size_t size = sizeof(thrm_smc_key_data_t);
    kern_return_t result = IOConnectCallStructMethod(gSMCConnection, 2, input, sizeof(thrm_smc_key_data_t), output, &size);
    if (result != kIOReturnSuccess) {
        return -1;
    }
    if (output->result != 0) {
        return -2;
    }
    return 0;
}

int thrmSMCRead(uint32_t key, void *outBytes, int *outSize, uint32_t *outType) {
    if (gSMCConnection == 0) {
        return -10;
    }
    thrm_smc_key_data_t input;
    thrm_smc_key_data_t output;
    memset(&input, 0, sizeof(input));
    memset(&output, 0, sizeof(output));

    input.key = key;
    input.data8 = 9; // READ_KEYINFO
    if (thrmSMCCall(&input, &output) != 0) {
        return -11;
    }
    if (output.keyInfo.dataSize == 0 || output.keyInfo.dataSize > 32) {
        return -12;
    }

    *outSize = (int)output.keyInfo.dataSize;
    *outType = output.keyInfo.dataType;

    memset(&input, 0, sizeof(input));
    memset(&output, 0, sizeof(output));
    input.key = key;
    input.keyInfo.dataSize = (uint32_t)*outSize;
    input.data8 = 5; // READ_BYTES
    if (thrmSMCCall(&input, &output) != 0) {
        return -13;
    }
    memcpy(outBytes, output.bytes, 32);
    return 0;
}
