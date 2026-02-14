#!/bin/bash
set -e

# ============================================================
# nekoray 一键编译脚本 (Windows MSYS2 MinGW64)
# 用法: 在 MSYS2 MinGW64 终端运行 bash build.sh
# ============================================================

SRC_ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$SRC_ROOT"

# ── Phase 0: 环境检测 ──────────────────────────────────────

if [ "$MSYSTEM" != "MINGW64" ]; then
    echo "错误: 请在 MSYS2 MinGW64 终端中运行此脚本"
    echo "  打开方式: 开始菜单 -> MSYS2 MinGW 64-bit"
    exit 1
fi

# 确保 MSYS2 工具链和系统工具都在 PATH 中
export PATH="/c/msys64/mingw64/bin:/c/msys64/usr/bin:$PATH"

# 修复临时目录权限 (非 MSYS2 终端下 TMP 可能指向 C:\WINDOWS)
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

# 自动探测 Go (Windows 安装的 Go 可能不在 MSYS2 PATH 中)
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
if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 22 ]); then
    echo "错误: Go 版本 $GO_VER 太低，需要 >= 1.22"
    exit 1
fi

echo "=== 环境检测通过 ==="
echo "  MSYS2 MinGW64, Go $GO_VER"

# ── Phase 1: 安装 MSYS2 依赖包 ─────────────────────────────

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
    mingw-w64-x86_64-zxing-cpp

# ── Phase 2: Git 子模块 ────────────────────────────────────

echo ""
echo "=== 初始化 Git 子模块 ==="
git submodule update --init --recursive

# ── Phase 3: 获取 Go 依赖源码 ──────────────────────────────

echo ""
echo "=== 获取 Go 依赖源码 ==="
bash libs/get_source.sh

# ── Phase 4: 编译 Go 后端 ──────────────────────────────────

echo ""
echo "=== 编译 Go 后端 ==="
export GOOS=windows
export GOARCH=amd64
# Go 在非标准 shell 下可能缺少默认路径
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

# ── Phase 6: 打包 ──────────────────────────────────────────

echo ""
echo "=== 打包到 dist/nekoray/ ==="

DIST="$SRC_ROOT/dist/nekoray"
rm -rf "$DIST"
mkdir -p "$DIST"

# 复制 GUI
cp build/nekobox.exe "$DIST/"

# windeployqt 收集 Qt DLL
pushd "$DIST" > /dev/null
windeployqt-qt5.exe nekobox.exe --no-compiler-runtime --no-opengl-sw 2>&1 || true
rm -rf translations
rm -f libEGL.dll libGLESv2.dll
popd > /dev/null

# Qt 插件 (windeployqt 在 MSYS2 下可能不复制插件目录)
QT_PLUGIN_DIR="/mingw64/share/qt5/plugins"
mkdir -p "$DIST/platforms" "$DIST/iconengines" "$DIST/imageformats" "$DIST/styles"
cp "$QT_PLUGIN_DIR/platforms/qwindows.dll" "$DIST/platforms/"
cp "$QT_PLUGIN_DIR/iconengines/"*.dll "$DIST/iconengines/" 2>/dev/null || true
cp "$QT_PLUGIN_DIR/imageformats/"*.dll "$DIST/imageformats/" 2>/dev/null || true
cp "$QT_PLUGIN_DIR/styles/"*.dll "$DIST/styles/" 2>/dev/null || true

# MinGW 运行时
for dll in libwinpthread-1.dll libgcc_s_seh-1.dll libstdc++-6.dll; do
    cp "/mingw64/bin/$dll" "$DIST/"
done

# 第三方库 DLL
cp /mingw64/bin/libZXing.dll "$DIST/" 2>/dev/null || cp /mingw64/bin/libzxing*.dll "$DIST/" 2>/dev/null || true
cp /mingw64/bin/libprotobuf*.dll "$DIST/"
cp /mingw64/bin/libyaml-cpp.dll "$DIST/"
cp /mingw64/bin/libabsl_*.dll "$DIST/"
cp /mingw64/bin/libutf8_validity.dll "$DIST/"

# OpenSSL (HTTPS 支持)
cp /mingw64/bin/libssl-3-x64.dll "$DIST/"
cp /mingw64/bin/libcrypto-3-x64.dll "$DIST/"

# Qt/图形库间接依赖 (windeployqt 不一定会复制)
for dll in libpng16-16.dll zlib1.dll libharfbuzz-0.dll libfreetype-6.dll \
           libgraphite2.dll libdouble-conversion.dll libpcre2-16-0.dll \
           libpcre2-8-0.dll libzstd.dll libmd4c.dll libglib-2.0-0.dll \
           libbz2-1.dll libiconv-2.dll libintl-8.dll libbrotlidec.dll \
           libbrotlicommon.dll; do
    cp "/mingw64/bin/$dll" "$DIST/" 2>/dev/null || true
done
# ICU 版本号随 MSYS2 更新变化，用通配符匹配
cp /mingw64/bin/libicuin*.dll "$DIST/" 2>/dev/null || true
cp /mingw64/bin/libicuuc*.dll "$DIST/" 2>/dev/null || true
cp /mingw64/bin/libicudt*.dll "$DIST/" 2>/dev/null || true

# Go 后端二进制
cp deployment/windows64/nekobox_core.exe "$DIST/" 2>/dev/null || true
cp deployment/windows64/updater.exe "$DIST/" 2>/dev/null || true

# 公共资源
cp res/public/* "$DIST/" 2>/dev/null || true

echo ""
echo "=== 编译完成 ==="
echo "输出目录: dist/nekoray/"
echo "运行: dist/nekoray/nekobox.exe"
