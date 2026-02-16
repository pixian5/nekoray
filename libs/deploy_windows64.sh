#!/bin/bash
set -e

# ============================================================
# MSVC 构建部署脚本
#
# 此脚本用于 MSVC + 静态链接第三方库 的构建路径。
# 第三方库 (protobuf, yaml-cpp, zxing 等) 通过 build_deps_all.sh
# 以 BUILD_SHARED_LIBS=OFF 静态编译，因此不需要复制它们的 DLL。
# 只需要 Qt DLL + OpenSSL。
#
# 如果使用 MSYS2 MinGW 动态链接构建，请使用 build.sh 或
# collect_dlls_mingw.sh 来自动收集 DLL。
# ============================================================

source libs/env_deploy.sh
DEST=$DEPLOYMENT/windows64
rm -rf $DEST
mkdir -p $DEST

#### copy exe ####
cp $BUILD/nekobox.exe $DEST

#### deploy qt & DLL runtime ####
pushd $DEST
windeployqt nekobox.exe --no-compiler-runtime --no-system-d3d-compiler --no-opengl-sw --verbose 2
rm -rf translations
rm -rf libEGL.dll libGLESv2.dll Qt6Pdf.dll

if [ "$DL_QT_VER" != "5.15" ]; then
  cp $SRC_ROOT/qtsdk/Qt/bin/libcrypto-3-x64.dll .
  cp $SRC_ROOT/qtsdk/Qt/bin/libssl-3-x64.dll .
fi

popd

#### prepare deployment ####
cp $BUILD/*.pdb $DEPLOYMENT
