#!/bin/bash
# 构建全部发布包：macOS / Windows / Linux / Android / iOS / 源码
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="1.0.0"
APP_NAME="InkVault"
DISPLAY_NAME="墨库 InkVault"
OUT="release"
DIST="dist"

echo "== 0/7 清理 =="
rm -rf "$OUT" "$DIST" build
mkdir -p "$OUT" "$DIST" build

echo "== 1/7 编译桌面二进制 =="
export GOFLAGS="-mod=mod" GOSUMDB=off CGO_ENABLED=0
go build -trimpath -ldflags "-s -w" -o "$DIST/inkvault-macos-arm64" .
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$DIST/inkvault-macos-amd64" .
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$DIST/inkvault-windows-amd64.exe" .
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$DIST/inkvault-linux-amd64" .

echo "== 2/7 图标 =="
ICON_SRC="web/icon-512.png"
mkdir -p build/icon.iconset
for s in 16 32 128 256 512; do
  sips -z $s $s "$ICON_SRC" --out "build/icon.iconset/icon_${s}x${s}.png" >/dev/null
  d=$((s*2))
  sips -z $d $d "$ICON_SRC" --out "build/icon.iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns build/icon.iconset -o build/icon.icns

make_app() {
  local binary="$1" appdir="$2"
  rm -rf "$appdir"
  mkdir -p "$appdir/Contents/MacOS" "$appdir/Contents/Resources"
  cp "$binary" "$appdir/Contents/MacOS/inkvault"
  chmod +x "$appdir/Contents/MacOS/inkvault"
  cp build/icon.icns "$appdir/Contents/Resources/icon.icns"
  cat > "$appdir/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>${DISPLAY_NAME}</string>
  <key>CFBundleDisplayName</key><string>${DISPLAY_NAME}</string>
  <key>CFBundleIdentifier</key><string>com.inkvault.desktop</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>inkvault</string>
  <key>CFBundleIconFile</key><string>icon</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSHumanReadableCopyright</key><string>InkVault · 本地优先的知识库</string>
</dict>
</plist>
PLIST
  codesign --force --deep --sign - "$appdir" >/dev/null 2>&1 || true
}

echo "== 3/7 macOS =="
make_app "$DIST/inkvault-macos-arm64" "build/${APP_NAME}.app"
mkdir -p build/dmg-arm
cp -R "build/${APP_NAME}.app" build/dmg-arm/
ln -sfn /Applications build/dmg-arm/Applications
hdiutil create -volname "${DISPLAY_NAME}" -srcfolder build/dmg-arm -ov -format UDZO \
  "$OUT/InkVault-${VERSION}-macOS-AppleSilicon.dmg" >/dev/null
(cd build && zip -qry "../$OUT/InkVault-${VERSION}-macOS-AppleSilicon.zip" "${APP_NAME}.app")

make_app "$DIST/inkvault-macos-amd64" "build/${APP_NAME}-Intel.app"
mkdir -p build/dmg-intel
cp -R "build/${APP_NAME}-Intel.app" build/dmg-intel/
ln -sfn /Applications build/dmg-intel/Applications
hdiutil create -volname "${DISPLAY_NAME}" -srcfolder build/dmg-intel -ov -format UDZO \
  "$OUT/InkVault-${VERSION}-macOS-Intel.dmg" >/dev/null
(cd build && zip -qry "../$OUT/InkVault-${VERSION}-macOS-Intel.zip" "${APP_NAME}-Intel.app")

echo "== 4/7 Windows / Linux =="
mkdir -p build/win
cp "$DIST/inkvault-windows-amd64.exe" build/win/InkVault.exe
cat > build/win/启动墨库.bat <<'BAT'
@echo off
chcp 65001 >nul
echo.
echo   墨库 InkVault - 本地优先的个人知识库
echo   启动后会自动打开浏览器；请保持此窗口开启。
echo.
start "" "InkVault.exe"
BAT
cat > build/win/使用说明.txt <<'TXT'
墨库 InkVault · Windows 版
=========================
1. 双击「启动墨库.bat」或 InkVault.exe。
2. 浏览器会自动打开 http://127.0.0.1:8790 并自动登录。
3. 访问码显示在启动窗口里，手机连接时使用。
4. 数据保存在程序旁边的 data 文件夹（vault.db + files + backups）。
   把整个文件夹复制走，就是完整备份。
5. 花生壳映射本机 8790 端口后，在「设置 → 公网地址」填入域名，
   手机和外地电脑即可访问同一套数据。
TXT
(cd build/win && zip -qr "../../$OUT/InkVault-${VERSION}-Windows-x64.zip" .)

mkdir -p build/linux
cp "$DIST/inkvault-linux-amd64" build/linux/inkvault
cat > build/linux/README.txt <<'TXT'
墨库 InkVault · Linux 版
./inkvault                    启动（http://127.0.0.1:8790）
./inkvault -data ~/InkVault   指定数据目录
TXT
(cd build/linux && zip -qr "../../$OUT/InkVault-${VERSION}-Linux-x64.zip" .)

echo "== 5/7 Android =="
./scripts/build_android.sh >/dev/null
mkdir -p build/android
cp "$DIST/InkVault-1.0.0.apk" build/android/
cat > build/android/安装说明.txt <<'TXT'
墨库 InkVault · Android 版
1. 把 InkVault-1.0.0.apk 传到手机并安装（允许「安装未知来源应用」）。
2. 首次启动可选择「离线模式」，或填写电脑端地址与访问码。
3. 离线数据保存在手机本机；在「设置 → 设备与同步」可连回电脑端双向同步。
TXT
(cd build/android && zip -qr "../../$OUT/InkVault-${VERSION}-Android.zip" .)

echo "== 6/7 iOS =="
rsync -a --delete --exclude 'icon.html' web/ ios/InkVault/www/
rm -rf ios/build
xcodebuild -project ios/InkVault.xcodeproj -target InkVault \
  -configuration Release -sdk iphoneos -arch arm64 \
  CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY="" \
  build >/dev/null
mkdir -p build/ios/Payload
cp -R ios/build/Release-iphoneos/InkVault.app build/ios/Payload/
(cd build/ios && zip -qry "../../dist/InkVault-${VERSION}-unsigned.ipa" Payload)
rm -rf build/ios/Payload
cp "dist/InkVault-${VERSION}-unsigned.ipa" build/ios/
cp -R ios/InkVault.xcodeproj build/ios/
cp -R ios/InkVault build/ios/InkVault
cat > build/ios/安装说明.txt <<'TXT'
墨库 InkVault · iOS 版
方式一（推荐，需 Mac + Xcode）：
  打开 InkVault.xcodeproj，在 Signing 里选择你的 Apple ID 团队，
  连接 iPhone 运行即可安装。
方式二（侧载）：
  用 Sideloadly / AltStore 打开 InkVault-1.0.0-unsigned.ipa，
  用你的 Apple ID 重新签名后安装。
TXT
(cd build/ios && zip -qry "../../$OUT/InkVault-${VERSION}-iOS.zip" .)

echo "== 7/7 源码 =="
mkdir -p build/src
rsync -a main.go go.mod go.sum README.md build/src/
rsync -a internal web scripts docs build/src/
mkdir -p build/src/android build/src/ios
cp android/AndroidManifest.xml build/src/android/
rsync -a android/src android/res build/src/android/
cp -R ios/InkVault.xcodeproj build/src/ios/
rsync -a --exclude 'www' ios/InkVault build/src/ios/InkVault
(cd build/src && zip -qry "../../$OUT/InkVault-${VERSION}-Source.zip" .)

echo
echo "发布产物："
ls -lh "$OUT"
