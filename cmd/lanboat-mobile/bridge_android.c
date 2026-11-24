//go:build android && cgo

#include <jni.h>
#include <stdint.h>

static JavaVM *vm;
static jclass bridge;

extern void LanboatHome(char *);
extern int LanboatAndroidAttached(int64_t, int);
extern void LanboatAndroidRevoked(int64_t);

JNIEXPORT void JNICALL Java_org_lanboat_mobile_LanboatBridge_init(JNIEnv *env, jclass cls, jstring home) {
    if (vm == NULL) {
        (*env)->GetJavaVM(env, &vm);
        bridge = (*env)->NewGlobalRef(env, cls);
    }
    const char *path = (*env)->GetStringUTFChars(env, home, NULL);
    if (path != NULL) {
        LanboatHome((char *)path);
        (*env)->ReleaseStringUTFChars(env, home, path);
    }
}

JNIEXPORT jboolean JNICALL Java_org_lanboat_mobile_LanboatBridge_attached(JNIEnv *env, jclass cls, jlong id, jint fd) {
    return LanboatAndroidAttached((int64_t)id, fd) ? JNI_TRUE : JNI_FALSE;
}

JNIEXPORT void JNICALL Java_org_lanboat_mobile_LanboatBridge_revoked(JNIEnv *env, jclass cls, jlong id) {
    LanboatAndroidRevoked((int64_t)id);
}

static JNIEnv *getEnv(int *attached) {
    JNIEnv *env = NULL;
    if (vm == NULL || bridge == NULL) return NULL;
    jint result = (*vm)->GetEnv(vm, (void **)&env, JNI_VERSION_1_6);
    if (result == JNI_EDETACHED) {
        if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) return NULL;
        *attached = 1;
    }
    return result == JNI_OK || *attached ? env : NULL;
}

static int finish(JNIEnv *env, int attached, jboolean result) {
    if ((*env)->ExceptionCheck(env)) {
        (*env)->ExceptionClear(env);
        result = JNI_FALSE;
    }
    if (attached) (*vm)->DetachCurrentThread(vm);
    return result == JNI_TRUE;
}

static int request(const char *name, int64_t id) {
    int attached = 0;
    JNIEnv *env = getEnv(&attached);
    if (env == NULL) return 0;
    jmethodID method = (*env)->GetStaticMethodID(env, bridge, name, "(J)Z");
    jboolean ok = method ? (*env)->CallStaticBooleanMethod(env, bridge, method, (jlong)id) : JNI_FALSE;
    return finish(env, attached, ok);
}
int lbAndroidStart(int64_t id) { return request("startRequest", id); }
int lbAndroidStop(int64_t id) { return request("stopRequest", id); }
int lbAndroidProtect(int64_t id, int fd) {
    int attached = 0;
    JNIEnv *env = getEnv(&attached);
    if (env == NULL) return 0;
    jmethodID method = (*env)->GetStaticMethodID(env, bridge, "protectSocket", "(JI)Z");
    jboolean ok = method ? (*env)->CallStaticBooleanMethod(env, bridge, method, (jlong)id, (jint)fd) : JNI_FALSE;
    return finish(env, attached, ok);
}
int lbAndroidOpen(const char *url) {
    int attached = 0;
    JNIEnv *env = getEnv(&attached);
    if (env == NULL) return 0;
    jmethodID method = (*env)->GetStaticMethodID(env, bridge, "openURL", "(Ljava/lang/String;)Z");
    jstring target = method ? (*env)->NewStringUTF(env, url) : NULL;
    jboolean ok = target ? (*env)->CallStaticBooleanMethod(env, bridge, method, target) : JNI_FALSE;
    if (target) (*env)->DeleteLocalRef(env, target);
    return finish(env, attached, ok);
}

extern void LanboatFileResult(char *);

JNIEXPORT void JNICALL Java_org_lanboat_mobile_LanboatBridge_fileResult(JNIEnv *env, jclass cls, jstring path) {
    const char *value = (*env)->GetStringUTFChars(env, path, NULL);
    if (value) {
        LanboatFileResult((char *)value);
        (*env)->ReleaseStringUTFChars(env, path, value);
    } else {
        LanboatFileResult("-");
    }
}

static int chooseFile(const char *path) {
    int attached = 0;
    JNIEnv *env = getEnv(&attached);
    if (env == NULL) return 0;
    jmethodID method = (*env)->GetStaticMethodID(env, bridge, "chooseFile", "(Ljava/lang/String;)Z");
    jstring value = method && path ? (*env)->NewStringUTF(env, path) : NULL;
    jboolean ok = method && (!path || value) ? (*env)->CallStaticBooleanMethod(env, bridge, method, value) : JNI_FALSE;
    if (value) (*env)->DeleteLocalRef(env, value);
    return finish(env, attached, ok);
}

int lbPickFile(void) { return chooseFile(NULL); }
int lbSaveFile(const char *path) { return chooseFile(path); }
