#!/bin/bash
# 构建墨库 Android APK（不依赖 Gradle，直接用 Android SDK 工具链）
set -euo pipefail
cd "$(dirname "$0")/.."

SDK="${ANDROID_HOME:-$HOME/android-sdk}"
BT="$SDK/build-tools/34.0.0"
PLATFORM="$SDK/platforms/android-34/android.jar"
JAVA="$SDK/jdk17/bin"
AAPT2="$BT/aapt2"
D8="$BT/d8"
ZIPALIGN="$BT/zipalign"
APKSIGNER="$BT/apksigner"

export JAVA_HOME="$JAVA"
export PATH="$JAVA:$PATH"

for f in "$AAPT2" "$D8" "$ZIPALIGN" "$APKSIGNER" "$PLATFORM" "$JAVA/javac"; do
  if [ ! -x "$f" ] && [ ! -f "$f" ]; then
    echo "缺少构建工具: $f" >&2
    exit 1
  fi
done

echo "1/6 同步前端资源"
rm -rf android/assets/www
mkdir -p android/assets/www
cp -R web/. android/assets/www/
rm -f android/assets/www/icon.html

echo "2/6 编译资源"
rm -rf android/build
mkdir -p android/build/gen android/build/classes android/build/dex
"$AAPT2" compile --dir android/res -o android/build/res.zip
"$AAPT2" link -o android/build/base.apk \
  -I "$PLATFORM" \
  --manifest android/AndroidManifest.xml \
  -R android/build/res.zip \
  --java android/build/gen \
  --min-sdk-version 24 --target-sdk-version 34 \
  --version-code 1 --version-name 1.0.0 \
  --auto-add-overlay

echo "3/6 编译 Java"
find android/src android/build/gen -name '*.java' > android/build/sources.txt
"$JAVA/javac" -encoding UTF-8 -source 8 -target 8 -classpath "$PLATFORM" \
  -d android/build/classes @"android/build/sources.txt" 2>&1 | grep -v "bootstrap class path" || true

echo "4/6 生成 dex"
"$D8" --lib "$PLATFORM" --min-api 24 --release --output android/build/dex \
  $(find android/build/classes -name '*.class')

echo "5/6 打包"
cp android/build/base.apk android/build/app.apk
(cd android/build && zip -q -j app.apk dex/classes.dex)
(cd android && zip -qr build/app.apk assets)
"$ZIPALIGN" -f 4 android/build/app.apk android/build/app-aligned.apk

echo "6/6 签名"
KEYSTORE="android/inkvault.keystore"
if [ ! -f "$KEYSTORE" ]; then
  "$JAVA/keytool" -genkeypair -keystore "$KEYSTORE" -alias inkvault \
    -keyalg RSA -keysize 2048 -validity 10000 \
    -storepass inkvault2026 -keypass inkvault2026 \
    -dname "CN=InkVault, OU=Personal, O=InkVault, L=Shanghai, C=CN" >/dev/null 2>&1
fi
mkdir -p dist
"$APKSIGNER" sign --ks "$KEYSTORE" --ks-pass pass:inkvault2026 --key-pass pass:inkvault2026 \
  --ks-key-alias inkvault --out dist/InkVault-1.0.0.apk android/build/app-aligned.apk
"$APKSIGNER" verify --print-certs dist/InkVault-1.0.0.apk | head -4

ls -lh dist/InkVault-1.0.0.apk
echo "APK 完成: dist/InkVault-1.0.0.apk"
