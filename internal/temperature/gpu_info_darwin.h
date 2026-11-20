#ifndef THRM_GPU_INFO_DARWIN_H
#define THRM_GPU_INFO_DARWIN_H

typedef struct {
    double powerW;
    int    powerOK;
    int    tempC;
    int    tempOK;
    char   model[256];
    int    modelOK;
} thrm_gpu_info_t;

int thrmReadGPUInfo(thrm_gpu_info_t *out);
const char *thrmGPUInfoModel(thrm_gpu_info_t *info);

#endif
