#!/bin/bash
set -e

# ============================================================
# nekoray 一键编译脚本 (Windows MSYS2 MinGW64)
# 用法: 在 MSYS2 MinGW64 终端运行 bash build.sh
#
# 功能:
#   1. 杀掉旧的 newbeeplus/xray 进程
#   2. 安装 MSYS2 依赖包
#   3. 初始化 Git 子模块 + Go 依赖
#   4. 编译 Go 后端 (newbeeplus_core.exe, updater.exe)
#   5. 编译 C++ GUI (newbeeplus.exe)
#   6. 复制运行依赖到 build/ (开发调试用)
#   7. 打包到 dist/nekoray/ (发布用)
#   8. 自动配置外部核心路径
#   9. 启动 newbeeplus
# ============================================================

SRC_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$SRC_ROOT"

# geodata 下载源 (共用)
GEODATA_URLS=(
    "geoip.db|https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip.db"
    "geosite.db|https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite.db"
    "geoip.dat|https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat"
    "geosite.dat|https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"
)

# 下载 geodata 到指定目录
download_geodata() {
    local dest="$1"
    for entry in "${GEODATA_URLS[@]}"; do
        local fname="${entry%%|*}"
        local url="${entry##*|}"
        if [ ! -f "$dest/$fname" ]; then
            echo "  下载 $fname -> $dest/"
            curl -fLso "$dest/$fname" "$url" || echo "  警告: 下载 $fname 失败"
        fi
    done
}

# 下载 Xray 到指定目录
download_xray() {
    local dest="$1"
    if [ ! -f "$dest/xray.exe" ]; then
        echo "  下载 Xray -> $dest/"
        local ver
        ver=$(curl -fLs "https://api.github.com/repos/XTLS/Xray-core/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+')
        if [ -n "$ver" ]; then
            curl -fLo /tmp/xray_dl.zip \
                "https://github.com/XTLS/Xray-core/releases/download/${ver}/Xray-windows-64.zip" \
                && unzip -o /tmp/xray_dl.zip xray.exe -d "$dest/" \
                && echo "  Xray $ver -> $dest/xray.exe" \
                || echo "  警告: 下载 Xray 失败"
            rm -f /tmp/xray_dl.zip
        else
            echo "  警告: 无法获取 Xray 版本号"
        fi
    fi
}

# 写入 xray 路径到 newbeeplus.json 配置
patch_xray_config() {
    local cfg="$1"
    local xray_exe="$2"
    if [ -f "$cfg" ] && [ -f "$xray_exe" ]; then
        local win_path
        win_path=$(cygpath -w "$xray_exe" | sed 's/\\/\\\\\\\\/g')
        if grep -q '"xray":""' "$cfg"; then
            sed -i "s|\"xray\":\"\"|\"xray\":\"${win_path}\"|" "$cfg"
            echo "  已写入 xray 路径: $(cygpath -w "$xray_exe")"
        else
            echo "  xray 路径已存在，跳过"
        fi
    fi
}

# ── Phase 0: 环境检测 ──────────────────────────────────────

if [ "$MSYSTEM" != "MINGW64" ]; then
    echo "错误: 请在 MSYS2 MinGW64 终端中运行此脚本"
    echo "  打开方式: 开始菜单 -> MSYS2 MinGW 64-bit"
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
    echo "错误: 未找到 Go 编译器，请先安装 Go >= 1.22"
    echo "  下载: https://go.dev/dl/"
    exit 1
fi

GO_VER=$(go version | grep -oP '\d+\.\d+' | head -1)
GO_MAJOR=$(echo "$GO_VER" | cut -d. -f1)
GO_MINOR=$(echo "$GO_VER" | cut -d. -f2)
if [ "$GO_MAJOR" -lt 1 ] || { [ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 22 ]; }; then
    echo "错误: Go 版本 $GO_VER 太低，需要 >= 1.22"
    exit 1
fi

echo "=== 环境检测通过 ==="
echo "  MSYS2 MinGW64, Go $GO_VER"

# ── Phase 1: 杀掉旧进程 ──────────────────────────────────

echo ""
echo "=== 结束旧进程 ==="
taskkill //F //IM newbeeplus.exe 2>/dev/null && echo "  已结束 newbeeplus.exe" || true
taskkill //F //IM newbeeplus_core.exe 2>/dev/null && echo "  已结束 newbeeplus_core.exe" || true
taskkill //F //IM xray.exe 2>/dev/null && echo "  已结束 xray.exe" || true
sleep 1

# ── Phase 2: 安装 MSYS2 依赖包 ─────────────────────────────

echo ""
echo "=== 安装 MSYS2 依赖包 ==="
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

# ── Phase 3: Git 子模块 + Go 依赖 ─────────────────────────

echo ""
echo "=== 初始化 Git 子模块 ==="
git submodule update --init --recursive

echo ""
echo "=== 获取 Go 依赖源码 ==="
bash libs/get_source.sh

# ── Phase 4: 编译 Go 后端 ──────────────────────────────────

echo ""
echo "=== 编译 Go 后端 ==="
export GOOS=windows
export GOARCH=amd64
export GOPATH="${GOPATH:-$HOME/go}"
export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"
export GOTMPDIR="${TEMP:-/tmp}"
export GOCACHE="$SRC_ROOT/.cache/go-build"
mkdir -p "$GOCACHE" "$GOMODCACHE"
bash libs/build_go.sh

# ── Phase 5: 编译 C++ GUI ──────────────────────────────────

echo ""
echo "=== 编译 C++ GUI ==="
mkdir -p build
cd build
cmake -G Ninja \
    -DCMAKE_BUILD_TYPE=Release \
    -DNKR_DISABLE_LIBS=ON \
    -DQT_VERSION_MAJOR=5 \
    ..
ninja
cd "$SRC_ROOT"

# ── Phase 6: 准备 build 目录运行环境 (开发调试用) ──────────

echo ""
echo "=== 准备 build/ 运行环境 ==="

# 复制 Go 后端
if [ -f deployment/windows64/newbeeplus_core.exe ]; then
    cp deployment/windows64/newbeeplus_core.exe build/
    echo "  newbeeplus_core.exe -> build/"
fi

# 下载 geodata 到 build/
download_geodata "build"

# 下载 Xray 到 build/xray_core/
mkdir -p build/xray_core
download_xray "build/xray_core"

# 写入 build 目录的 xray 配置
patch_xray_config "build/config/groups/newbeeplus.json" "$SRC_ROOT/build/xray_core/xray.exe"

# ── Phase 7: 打包到 dist/nekoray/ (发布用) ─────────────────

echo ""
echo "=== 打包到 dist/nekoray/ ==="

DIST="$SRC_ROOT/dist/nekoray"
rm -rf "$DIST"
mkdir -p "$DIST"

# 复制 GUI
cp build/newbeeplus.exe "$DIST/"

# windeployqt 收集 Qt DLL
pushd "$DIST" > /dev/null
windeployqt-qt5.exe newbeeplus.exe --no-compiler-runtime --no-opengl-sw 2>&1 || true
rm -rf translations
rm -f libEGL.dll libGLESv2.dll
popd > /dev/null

# Qt 插件
QT_PLUGIN_DIR="/mingw64/share/qt5/plugins"
mkdir -p "$DIST/platforms" "$DIST/iconengines" "$DIST/imageformats" "$DIST/styles"
cp "$QT_PLUGIN_DIR/platforms/qwindows.dll" "$DIST/platforms/"
cp "$QT_PLUGIN_DIR/iconengines/"*.dll "$DIST/iconengines/" 2>/dev/null || true
cp "$QT_PLUGIN_DIR/imageformats/"*.dll "$DIST/imageformats/" 2>/dev/null || true
cp "$QT_PLUGIN_DIR/styles/"*.dll "$DIST/styles/" 2>/dev/null || true

# 自动收集所有 MinGW DLL 依赖
bash "$SRC_ROOT/libs/collect_dlls_mingw.sh" "$DIST"

# Go 后端二进制
cp deployment/windows64/newbeeplus_core.exe "$DIST/" 2>/dev/null || true
cp deployment/windows64/updater.exe "$DIST/" 2>/dev/null || true

# 公共资源
cp res/public/* "$DIST/" 2>/dev/null || true

# 下载 geodata 到 dist/
download_geodata "$DIST"

# 下载代理核心到 dist/
echo ""
echo "=== 下载代理核心 (dist) ==="

# sing-box (newbeeplus_core) - 仅在编译的不存在时下载
if [ ! -f "$DIST/newbeeplus_core.exe" ]; then
    echo "  下载 sing-box (newbeeplus_core) ..."
    SINGBOX_VER=$(curl -fLs "https://api.github.com/repos/SagerNet/sing-box/releases/latest" | grep -oP '"tag_name":\s*"\K[^"]+')
    if [ -n "$SINGBOX_VER" ]; then
        curl -fLo /tmp/sing-box.tar.gz \
            "https://github.com/SagerNet/sing-box/releases/download/${SINGBOX_VER}/sing-box-${SINGBOX_VER#v}-windows-amd64.tar.gz" \
            && tar -xzf /tmp/sing-box.tar.gz -C /tmp --wildcards '*/sing-box.exe' \
            && find /tmp -name 'sing-box.exe' -exec cp {} "$DIST/newbeeplus_core.exe" \; \
            && echo "  sing-box $SINGBOX_VER -> newbeeplus_core.exe" \
            || echo "  警告: 下载 sing-box 失败，尝试 zip 格式 ..."
        if [ ! -f "$DIST/newbeeplus_core.exe" ]; then
            curl -fLo /tmp/sing-box.zip \
                "https://github.com/SagerNet/sing-box/releases/download/${SINGBOX_VER}/sing-box-${SINGBOX_VER#v}-windows-amd64.zip" \
                && unzip -o /tmp/sing-box.zip '*/sing-box.exe' -d /tmp \
                && find /tmp -name 'sing-box.exe' -exec cp {} "$DIST/newbeeplus_core.exe" \; \
                && echo "  sing-box $SINGBOX_VER -> newbeeplus_core.exe" \
                || echo "  警告: 下载 sing-box 失败"
        fi
    else
        echo "  警告: 无法获取 sing-box 版本号"
    fi
fi

# Xray
download_xray "$DIST"

# ── Phase 8: 配置 dist 目录外部核心路径 ────────────────────

echo ""
echo "=== 配置 dist 外部核心路径 ==="
patch_xray_config "$DIST/config/groups/newbeeplus.json" "$DIST/xray.exe"

# ── 完成，启动 ────────────────────────────────────────────

echo ""
echo "=== 编译完成 ==="
echo "  build 目录: $SRC_ROOT/build/  (开发调试)"
echo "  dist  目录: $DIST/  (发布打包)"
echo ""
echo "启动 newbeeplus ..."
"$DIST/newbeeplus.exe" &
