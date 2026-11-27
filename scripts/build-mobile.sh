#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off
target=${1:?android or ios}
out=$(python3 -c 'import os,sys; print(os.path.abspath(sys.argv[1]))' "${2:?output path}")
go mod download github.com/zatxm/fego/v26
fegodir=$(go list -mod=readonly -m -f '{{.Dir}}' github.com/zatxm/fego/v26)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$(dirname "$out")"
bundle_id=${LANBOAT_MOBILE_ID:-org.lanboat.mobile}
version=${LANBOAT_VERSION:-1.0.0}
build_number=${LANBOAT_BUILD_NUMBER:-1}
[[ $version =~ ^[0-9]+(\.[0-9]+){2}$ && $build_number =~ ^[1-9][0-9]*$ && $bundle_id =~ ^[A-Za-z][A-Za-z0-9]*(\.[A-Za-z][A-Za-z0-9]*)+$ ]]
build=(go build -mod=readonly -trimpath -buildvcs=true -ldflags '-s -w')

case $target in
android)
    arch=${ANDROID_ARCH:-arm64}
    case $arch in
        arm64) abi=arm64-v8a; triple=aarch64 ;;
        amd64) abi=x86_64; triple=x86_64 ;;
        *) printf 'Unsupported Android architecture: %s\n' "$arch" >&2; exit 2 ;;
    esac
    sdk=${ANDROID_HOME:?set ANDROID_HOME}
    tools=${ANDROID_BUILD_TOOLS:-"$sdk/build-tools/36.0.0"}
    ndk=${ANDROID_NDK_HOME:-"$sdk/ndk/28.2.13676358"}
    export PATH="${JAVA_HOME:?set JAVA_HOME}/bin:$PATH"
    jarfile="$sdk/platforms/android-36/android.jar"
    case $(uname -s) in
        Linux) host=linux-x86_64 ;;
        Darwin) host=darwin-x86_64 ;;
        *) exit 2 ;;
    esac
    mkdir -p "$tmp/classes" "$tmp/apk" "$tmp/src/org/fego"
    cp "$fegodir"/mobile/android/org/fego/*.java "$tmp/src/org/fego/"
    chmod u+w "$tmp/src/org/fego/FegoActivity.java"
    python3 - "$tmp/src/org/fego/FegoActivity.java" <<'PY'
import pathlib, sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
start = '    start();'
callback = '  @Override\n  public void onBackPressed()'
if text.count(start) != 1 or text.count(callback) != 1:
    raise SystemExit('Fego activity hooks no longer match the pinned mobile host')
text = text.replace(start, '    org.lanboat.mobile.LanboatBridge.attach(this);\n' + start)
text = text.replace(callback, '''  @Override
  protected void onActivityResult(int request, int result, android.content.Intent data) {
    super.onActivityResult(request, result, data);
    org.lanboat.mobile.LanboatBridge.activityResult(request, result, data);
  }

''' + callback)
path.write_text(text)
PY
    lib="$tmp/apk/lib/$abi"
    mkdir -p "$lib"
    GOOS=android GOARCH="$arch" CGO_ENABLED=1 CC="$ndk/toolchains/llvm/prebuilt/$host/bin/$triple-linux-android23-clang" \
        "${build[@]}" -buildmode=c-shared -o "$lib/libfego.so" ./cmd/lanboat-mobile
    rm -f "$lib/libfego.h"
    javac --release 8 -classpath "$jarfile" -d "$tmp/classes" \
        "$tmp"/src/org/fego/*.java mobile/android/org/lanboat/mobile/*.java
    jar cf "$tmp/classes.jar" -C "$tmp/classes" .
    "$tools/d8" --min-api 23 --lib "$jarfile" --output "$tmp/apk" "$tmp/classes.jar"
    manifest_opts=()
    if [[ -z ${ANDROID_KEYSTORE:-} ]]; then
        manifest_opts+=(--debug-mode)
        keytool -genkeypair -keystore "$tmp/debug.keystore" -storepass android -keypass android \
            -alias androiddebugkey -dname 'CN=Android Debug' -keyalg RSA -keysize 2048 -validity 10000
        sign=(--ks "$tmp/debug.keystore" --ks-pass pass:android --key-pass pass:android)
    else
        : "${ANDROID_KEY_ALIAS:?set ANDROID_KEY_ALIAS}" "${ANDROID_KEYSTORE_PASSWORD:?set ANDROID_KEYSTORE_PASSWORD}"
        export ANDROID_KEY_PASSWORD=${ANDROID_KEY_PASSWORD:-$ANDROID_KEYSTORE_PASSWORD}
        sign=(--ks "$ANDROID_KEYSTORE" --ks-key-alias "$ANDROID_KEY_ALIAS" --ks-pass env:ANDROID_KEYSTORE_PASSWORD --key-pass env:ANDROID_KEY_PASSWORD)
    fi
    mkdir -p "$tmp/res/mipmap-xxxhdpi"
    cp packaging/icons/256x256/lanboat.png "$tmp/res/mipmap-xxxhdpi/lanboat.png"
    "$tools/aapt2" compile --dir "$tmp/res" -o "$tmp/resources.zip"
    "$tools/aapt2" link -I "$jarfile" --manifest mobile/android/AndroidManifest.xml \
        -R "$tmp/resources.zip" \
        --rename-manifest-package "$bundle_id" --version-name "$version" --version-code "$build_number" \
        --replace-version "${manifest_opts[@]}" -o "$tmp/base.apk"
    (cd "$tmp/apk" && zip -q -0 -r "$tmp/base.apk" classes.dex lib)
    "$tools/zipalign" -P 16 -f 4 "$tmp/base.apk" "$tmp/aligned.apk"
    "$tools/apksigner" sign "${sign[@]}" --out "$tmp/signed.apk" "$tmp/aligned.apk"
    "$tools/apksigner" verify "$tmp/signed.apk"
    if [[ $(unzip -Z1 "$tmp/signed.apk" 'lib/*/*.so') != "lib/$abi/libfego.so" ]]; then
        printf 'APK does not contain exactly one native library for %s\n' "$abi" >&2
        exit 1
    fi
    cp "$tmp/signed.apk" "$out"
    ;;
ios)
    sdk=${APPLE_SDK:-iphoneos}
    deployment=${IOS_MIN:-15.0}
    case $sdk in
        iphoneos) target_triple="arm64-apple-ios$deployment" ;;
        iphonesimulator) target_triple="arm64-apple-ios$deployment-simulator" ;;
        *) exit 2 ;;
    esac
    sysroot=$(xcrun --sdk "$sdk" --show-sdk-path)
    clang=$(xcrun --sdk "$sdk" -f clang)
    export GOOS=ios GOARCH=arm64 CGO_ENABLED=1
    export CC="$clang"
    export CGO_CFLAGS="-isysroot $sysroot -target $target_triple -fobjc-arc"
    export CGO_LDFLAGS="-isysroot $sysroot -target $target_triple"
    "${build[@]}" -buildmode=c-archive -o "$tmp/Lanboat.a" ./cmd/lanboat-mobile
    "${build[@]}" -buildmode=c-archive -o "$tmp/PacketTunnel.a" ./cmd/lanboat-packet-tunnel
    app="$tmp/Payload/Lanboat.app"
    tunnel="$app/PlugIns/PacketTunnel.appex"
    mkdir -p "$tunnel"
    frameworks=(-framework Foundation -framework NetworkExtension -framework Security -framework CoreFoundation -lresolv)
    common=(-isysroot "$sysroot" -target "$target_triple" -fobjc-arc)
    "$clang" "${common[@]}" mobile/ios/App.m -I"$fegodir/mobile/ios" "$tmp/Lanboat.a" \
        "${frameworks[@]}" -framework UIKit -framework UniformTypeIdentifiers -framework QuartzCore -framework CoreGraphics -o "$app/Lanboat"
    "$clang" "${common[@]}" -fapplication-extension -Wl,-e,_NSExtensionMain mobile/ios/PacketTunnel.m \
        "$tmp/PacketTunnel.a" "${frameworks[@]}" -o "$tunnel/PacketTunnel"

    python3 - "$fegodir/mobile/ios/Info.plist" "$app" "$bundle_id" "$version" "$build_number" "$deployment" "$sdk" <<'PY'
import pathlib, plistlib, subprocess, sys

template, bundle, identifier, version, build, minimum, sdk = sys.argv[1:]
platforms = ['iPhoneSimulator' if sdk == 'iphonesimulator' else 'iPhoneOS']
root = pathlib.Path(bundle)
with open(template, 'rb') as f:
    config = plistlib.load(f)
config.update(CFBundleIdentifier=identifier, CFBundleExecutable='Lanboat', CFBundleName='Lanboat',
              CFBundleVersion=build, CFBundleShortVersionString=version, MinimumOSVersion=minimum,
              CFBundleSupportedPlatforms=platforms)
for key, icons in (
    ('CFBundleIcons', [('LanboatIcon@2x.png', 120), ('LanboatIcon@3x.png', 180)]),
    ('CFBundleIcons~ipad', [('LanboatPadIcon@2x.png', 152), ('LanboatPadProIcon@2x.png', 167)]),
):
    config[key] = {'CFBundlePrimaryIcon': {'CFBundleIconFiles': [name for name, _ in icons]}}
    for name, size in icons:
        subprocess.run(['sips', '-z', str(size), str(size), 'packaging/icons/256x256/lanboat.png',
                        '--out', str(root / name)], check=True, stdout=subprocess.DEVNULL)
scene = config['UIApplicationSceneManifest']['UISceneConfigurations']['UIWindowSceneSessionRoleApplication'][0]
scene.update(UISceneConfigurationName='Lanboat', UISceneDelegateClassName='LanboatScene')
with (root / 'Info.plist').open('wb') as f:
    plistlib.dump(config, f)
with (root / 'PlugIns/PacketTunnel.appex/Info.plist').open('wb') as f:
    plistlib.dump(dict(CFBundleIdentifier=identifier + '.PacketTunnel', CFBundleExecutable='PacketTunnel',
                       CFBundleName='Lanboat VPN', CFBundlePackageType='XPC!', CFBundleVersion=build,
                       CFBundleShortVersionString=version, MinimumOSVersion=minimum,
                       CFBundleSupportedPlatforms=platforms, UIDeviceFamily=config['UIDeviceFamily'], LSRequiresIPhoneOS=True,
                       NSExtension=dict(NSExtensionPointIdentifier='com.apple.networkextension.packet-tunnel',
                                        NSExtensionPrincipalClass='LanboatPacketTunnel')), f)
PY
    if [[ $sdk == iphoneos ]]; then
        : "${IOS_IDENTITY:?set IOS_IDENTITY}" "${IOS_APP_PROFILE:?set IOS_APP_PROFILE}" "${IOS_TUNNEL_PROFILE:?set IOS_TUNNEL_PROFILE}"
        for component in app tunnel; do
            if [[ $component == app ]]; then
                path=$app profile=$IOS_APP_PROFILE identifier=$bundle_id
            else
                path=$tunnel profile=$IOS_TUNNEL_PROFILE identifier="$bundle_id.PacketTunnel"
            fi
            security cms -D -i "$profile" > "$tmp/profile.plist"
            python3 - "$tmp/profile.plist" "$identifier" "$tmp/$component.entitlements" <<'PY'
import plistlib, sys

src, identifier, target = sys.argv[1:]
with open(src, 'rb') as f:
    allowed = plistlib.load(f)['Entitlements']
app_id = allowed['application-identifier']
network = 'com.apple.developer.networking.networkextension'
if app_id.split('.', 1)[-1] != identifier or 'packet-tunnel-provider' not in allowed.get(network, []):
    raise SystemExit(f'Provisioning profile does not allow a packet tunnel for {identifier}')
keys = ('application-identifier', 'com.apple.developer.team-identifier', 'get-task-allow', network)
with open(target, 'wb') as f:
    ent = {key: allowed[key] for key in keys if key in allowed}
    ent[network] = ['packet-tunnel-provider']
    plistlib.dump(ent, f)
PY
            cp "$profile" "$path/embedded.mobileprovision"
        done
        codesign --force --sign "$IOS_IDENTITY" --entitlements "$tmp/tunnel.entitlements" "$tunnel"
        codesign --force --sign "$IOS_IDENTITY" --entitlements "$tmp/app.entitlements" "$app"
    else
        codesign --force --sign - "$tunnel"
        codesign --force --sign - "$app"
    fi
    codesign --verify --deep --strict "$app"
    (cd "$tmp" && zip -q -r "Lanboat.ipa" Payload)
    cp "$tmp/Lanboat.ipa" "$out"
    ;;
*) echo "usage: $0 android|ios path/to/package" >&2; exit 2 ;;
esac
printf '%s\n' "$out"
