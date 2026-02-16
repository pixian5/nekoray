#!/bin/bash
set -e

# ============================================================
# nekoray 涓€閿紪璇戣剼鏈?(Windows MSYS2 MinGW64)
# 鐢ㄦ硶: 鍦?MSYS2 MinGW64 缁堢杩愯 bash build.sh
#
# 鍔熻兘:
#   1. 鏉€鎺夋棫鐨?newbeeplus/xray 杩涚▼
#   2. 瀹夎 MSYS2 渚濊禆鍖?#   3. 鍒濆鍖?Git 瀛愭ā鍧?+ Go 渚濊禆
#   4. 缂栬瘧 Go 鍚庣 (newbeeplus_core.exe, updater.exe)
#   5. 缂栬瘧 C++ GUI (newbeeplus.exe)
#   6. 澶嶅埗杩愯渚濊禆鍒?build/ (寮€鍙戣皟璇曠敤)
#   7. 鎵撳寘鍒?dist/nekoray/ (鍙戝竷鐢?
#   8. 鑷姩閰嶇疆澶栭儴鏍稿績璺緞
#   9. 鍚姩 newbeeplus
# ============================================================

SRC_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$SRC_ROOT"

# geodata 涓嬭浇婧?(鍏辩敤)
GEODATA_URLS=(
    "geoip.db|https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip.db"
    "geosite.db|https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite.db"
    "geoip.dat|https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat"
    "geosite.dat|https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"
)

# Download geodata to destination
download_geodata() {
    local dest="$1"
    for entry in "${GEODATA_URLS[@]}"; do
        local fname="${entry%%|*}"
        local url="${entry##*|}"
        if [ ! -f "$dest/$fname" ]; then
            echo "  涓嬭浇 $fname -> $dest/"
            curl -fLso "$dest/$fname" "$url" || echo "  璀﹀憡: 涓嬭浇 $fname 澶辫触"
        fi
    done
}

# Download Xray to destination
download_xray() {
    local dest="$1"
    if [ ! -f "$dest/xray.exe" ]; then
        echo "  涓嬭浇 Xray -> $dest/"
        rm -f /tmp/xray_dl.zip
        local ver
        ver=$(curl -fLs "https://api.github.com/repos/XTLS/Xray-core/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+')
        if [ -n "$ver" ]; then
            curl -fLo /tmp/xray_dl.zip \
                "https://github.com/XTLS/Xray-core/releases/download/${ver}/Xray-windows-64.zip" \
                && unzip -o /tmp/xray_dl.zip xray.exe -d "$dest/" \
                && echo "  Xray $ver -> $dest/xray.exe" \
                || echo "  璀﹀憡: 涓嬭浇 Xray 澶辫触"
            rm -f /tmp/xray_dl.zip
        else
            echo "  Warning: failed to fetch Xray version"
        fi
    fi
}

download_singbox() {
    local dest="$1"
    if [ -f "$dest/sing-box.exe" ]; then
        return
    fi

    echo "  Download sing-box -> $dest/"
    rm -f /tmp/sing-box.tar.gz /tmp/sing-box.zip
    local ver
    ver=$(curl -fLs "https://api.github.com/repos/SagerNet/sing-box/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+')
    if [ -n "$ver" ]; then
        local tmp_dir
        tmp_dir="/tmp/sing-box-${ver}-$$"
        rm -rf "$tmp_dir"
        mkdir -p "$tmp_dir"
        curl -fLo /tmp/sing-box.tar.gz \
            "https://github.com/SagerNet/sing-box/releases/download/${ver}/sing-box-${ver#v}-windows-amd64.tar.gz" \
            && tar -xzf /tmp/sing-box.tar.gz -C "$tmp_dir" --wildcards '*/sing-box.exe' \
            && find "$tmp_dir" -name 'sing-box.exe' -exec cp {} "$dest/sing-box.exe" \; \
            && echo "  sing-box $ver -> $dest/sing-box.exe" \
            || echo "  Warning: failed to download sing-box tar.gz, trying zip..."
        if [ ! -f "$dest/sing-box.exe" ]; then
            curl -fLo /tmp/sing-box.zip \
                "https://github.com/SagerNet/sing-box/releases/download/${ver}/sing-box-${ver#v}-windows-amd64.zip" \
                && unzip -o /tmp/sing-box.zip '*/sing-box.exe' -d "$tmp_dir" \
                && find "$tmp_dir" -name 'sing-box.exe' -exec cp {} "$dest/sing-box.exe" \; \
                && echo "  sing-box $ver -> $dest/sing-box.exe" \
                || echo "  Warning: failed to download sing-box zip"
        fi
        rm -rf "$tmp_dir"
    else
        echo "  Warning: failed to fetch sing-box version"
    fi
    rm -f /tmp/sing-box.tar.gz /tmp/sing-box.zip
}

# 鍐欏叆 xray 璺緞鍒?newbeeplus.json 閰嶇疆
patch_xray_config() {
    local cfg="$1"
    local xray_exe="$2"
    if [ -f "$cfg" ] && [ -f "$xray_exe" ]; then
        local win_path
        win_path=$(cygpath -w "$xray_exe" | sed 's/\\/\\\\\\\\/g')
        if grep -q '"xray":""' "$cfg"; then
            sed -i "s|\"xray\":\"\"|\"xray\":\"${win_path}\"|" "$cfg"
            echo "  宸插啓鍏?xray 璺緞: $(cygpath -w "$xray_exe")"
        else
            echo "  xray 璺緞宸插瓨鍦紝璺宠繃"
        fi
    fi
}

# 鈹€鈹€ Phase 0: 鐜妫€娴?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

if [ "$MSYSTEM" != "MINGW64" ]; then
    echo "閿欒: 璇峰湪 MSYS2 MinGW64 缁堢涓繍琛屾鑴氭湰"
    echo "  鎵撳紑鏂瑰紡: 寮€濮嬭彍鍗?-> MSYS2 MinGW 64-bit"
    exit 1
fi

export PATH="/c/msys64/mingw64/bin:/c/msys64/usr/bin:$PATH"

_user_tmp="/c/Users/$USERNAME/AppData/Local/Temp"
if [ ! -w "${TMP:-/nonexistent}" ] || [ "${TMP}" = "/tmp" ]; then
    if [ -d "$_user_tmp" ]; then
        export TMP="$_user_tmp"
        export TEMP="$_user_tmp"
    else
        mkdir -p /tmp
        export TMP="/tmp"
        export TEMP="/tmp"
    fi
fi

if ! command -v go &>/dev/null; then
    for p in "/c/Program Files/Go/bin" "/c/Go/bin" "$USERPROFILE/go/bin"; do
        if [ -x "$p/go.exe" ] || [ -x "$p/go" ]; then
            export PATH="$p:$PATH"
            break
        fi
    done
fi
if ! command -v go &>/dev/null; then
    echo "閿欒: 鏈壘鍒?Go 缂栬瘧鍣紝璇峰厛瀹夎 Go >= 1.22"
    echo "  涓嬭浇: https://go.dev/dl/"
    exit 1
fi

GO_VER=$(go version | grep -oP '\d+\.\d+' | head -1)
GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 22 ]; }; then
    echo "閿欒: Go 鐗堟湰 $GO_VER 澶綆锛岄渶瑕?>= 1.22"
    exit 1
fi

echo "=== 鐜妫€娴嬮€氳繃 ==="
echo "  MSYS2 MinGW64, Go $GO_VER"

# 鈹€鈹€ Phase 1: 鏉€鎺夋棫杩涚▼ 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 缁撴潫鏃ц繘绋?==="
taskkill //F //IM newbeeplus.exe 2>/dev/null && echo "  宸茬粨鏉?newbeeplus.exe" || true
taskkill //F //IM newbeeplus_core.exe 2>/dev/null && echo "  宸茬粨鏉?newbeeplus_core.exe" || true
taskkill //F //IM xray.exe 2>/dev/null && echo "  宸茬粨鏉?xray.exe" || true
sleep 1

# 鈹€鈹€ Phase 2: 瀹夎 MSYS2 渚濊禆鍖?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 瀹夎 MSYS2 渚濊禆鍖?==="
pacman -S --noconfirm --needed \
    mingw-w64-x86_64-toolchain \
    mingw-w64-x86_64-cmake \
    mingw-w64-x86_64-ninja \
    mingw-w64-x86_64-qt5-base \
    mingw-w64-x86_64-qt5-svg \
    mingw-w64-x86_64-qt5-tools \
    mingw-w64-x86_64-protobuf \
    mingw-w64-x86_64-yaml-cpp \
    mingw-w64-x86_64-zxing-cpp \
    mingw-w64-x86_64-ntldd

# 鈹€鈹€ Phase 3: Git 瀛愭ā鍧?+ Go 渚濊禆 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 鍒濆鍖?Git 瀛愭ā鍧?==="
git submodule update --init --recursive

echo ""
echo "=== 鑾峰彇 Go 渚濊禆婧愮爜 ==="
bash libs/get_source.sh

# 鈹€鈹€ Phase 4: 缂栬瘧 Go 鍚庣 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 缂栬瘧 Go 鍚庣 ==="
export GOOS=windows
export GOARCH=amd64
export GOPATH="${GOPATH:-$HOME/go}"
export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"
export GOTMPDIR="${TEMP:-/tmp}"
export GOCACHE="$SRC_ROOT/.cache/go-build"
mkdir -p "$GOCACHE" "$GOMODCACHE"
bash libs/build_go.sh

# 鈹€鈹€ Phase 5: 缂栬瘧 C++ GUI 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 缂栬瘧 C++ GUI ==="
mkdir -p build
cd build
cmake -G Ninja \
    -DCMAKE_BUILD_TYPE=Release \
    -DNKR_DISABLE_LIBS=ON \
    -DQT_VERSION_MAJOR=5 \
    ..
ninja
cd "$SRC_ROOT"

# 鈹€鈹€ Phase 6: 鍑嗗 build 鐩綍杩愯鐜 (寮€鍙戣皟璇曠敤) 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 鍑嗗 build/ 杩愯鐜 ==="

# 澶嶅埗 Go 鍚庣
mkdir -p build/core
if [ -f deployment/windows64/newbeeplus_core.exe ]; then
    cp deployment/windows64/newbeeplus_core.exe build/core/
    echo "  newbeeplus_core.exe -> build/core/"
fi

# 涓嬭浇 geodata 鍒?build/
download_geodata "build"

# 涓嬭浇 Xray 鍒?build/core/
download_xray "build/core"
download_singbox "build/core"

# 鍐欏叆 build 鐩綍鐨?xray 閰嶇疆
patch_xray_config "build/config/groups/newbeeplus.json" "$SRC_ROOT/build/core/xray.exe"

# 鈹€鈹€ Phase 7: 鎵撳寘鍒?dist/nekoray/ (鍙戝竷鐢? 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 鎵撳寘鍒?dist/nekoray/ ==="

DIST="$SRC_ROOT/dist/nekoray"
rm -rf "$DIST"
mkdir -p "$DIST/core"

# 澶嶅埗 GUI
cp build/newbeeplus.exe "$DIST/"

# windeployqt 鏀堕泦 Qt DLL
pushd "$DIST" > /dev/null
windeployqt-qt5.exe newbeeplus.exe --no-compiler-runtime --no-opengl-sw 2>&1 || true
rm -rf translations
rm -f libEGL.dll libGLESv2.dll
popd > /dev/null

# Qt 鎻掍欢
QT_PLUGIN_DIR="/mingw64/share/qt5/plugins"
mkdir -p "$DIST/platforms" "$DIST/iconengines" "$DIST/imageformats" "$DIST/styles"
cp "$QT_PLUGIN_DIR/platforms/qwindows.dll" "$DIST/platforms/"
cp "$QT_PLUGIN_DIR/iconengines/"*.dll "$DIST/iconengines/" 2>/dev/null || true
cp "$QT_PLUGIN_DIR/imageformats/"*.dll "$DIST/imageformats/" 2>/dev/null || true
cp "$QT_PLUGIN_DIR/styles/"*.dll "$DIST/styles/" 2>/dev/null || true

# 鑷姩鏀堕泦鎵€鏈?MinGW DLL 渚濊禆
bash "$SRC_ROOT/libs/collect_dlls_mingw.sh" "$DIST"

# core binaries
cp deployment/windows64/newbeeplus_core.exe "$DIST/core/" 2>/dev/null || true
cp deployment/windows64/updater.exe "$DIST/" 2>/dev/null || true

# 鍏叡璧勬簮
cp res/public/* "$DIST/" 2>/dev/null || true

# 涓嬭浇 geodata 鍒?dist/
download_geodata "$DIST"

# 涓嬭浇浠ｇ悊鏍稿績鍒?dist/
echo ""
echo "=== 涓嬭浇浠ｇ悊鏍稿績 (dist) ==="

# sing-box
download_singbox "$DIST/core"

# Xray
download_xray "$DIST/core"

# 鈹€鈹€ Phase 8: 閰嶇疆 dist 鐩綍澶栭儴鏍稿績璺緞 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 閰嶇疆 dist 澶栭儴鏍稿績璺緞 ==="
patch_xray_config "$DIST/config/groups/newbeeplus.json" "$DIST/core/xray.exe"

# 鈹€鈹€ 瀹屾垚锛屽惎鍔?鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

echo ""
echo "=== 缂栬瘧瀹屾垚 ==="
echo "  build 鐩綍: $SRC_ROOT/build/  (寮€鍙戣皟璇?"
echo "  dist  鐩綍: $DIST/  (鍙戝竷鎵撳寘)"
echo ""
echo "鍚姩 newbeeplus ..."
"$DIST/newbeeplus.exe" &

