# Windows + MSYS2 MinGW 64-bit 编译与运行 nekoray 项目全流程

## 1. 安装环境
- 安装 MSYS2（https://www.msys2.org/），并更新系统：
  ```sh
  pacman -Syu
  ```

## 2. 安装所有依赖
在 MSYS2 MinGW 64-bit 终端执行：
```sh
pacman -S --noconfirm \
  mingw-w64-x86_64-toolchain \
  mingw-w64-x86_64-cmake \
  mingw-w64-x86_64-qt5 \
  mingw-w64-x86_64-protobuf \
  mingw-w64-x86_64-yaml-cpp \
  mingw-w64-x86_64-zxing-cpp \
  mingw-w64-x86_64-glib2 \
  mingw-w64-x86_64-zstd \
  mingw-w64-x86_64-bzip2 \
  mingw-w64-x86_64-libiconv \
  mingw-w64-x86_64-libintl \
  mingw-w64-x86_64-pcre \
  mingw-w64-x86_64-pcre2 \
  mingw-w64-x86_64-brotli \
  mingw-w64-x86_64-expat \
  mingw-w64-x86_64-openssl \
  mingw-w64-x86_64-md4c
```

## 3. 拉取子模块
```sh
git submodule update --init --recursive
```

## 4. 编译项目
```sh
cmake -G "MinGW Makefiles" -S . -B build
cmake --build build
```

## 5. 复制所有运行时依赖 DLL
```sh
cp /mingw64/bin/libwinpthread-1.dll ./build/
cp /mingw64/bin/libgcc_s_seh-1.dll ./build/
cp /mingw64/bin/libstdc++-6.dll ./build/
cp /mingw64/bin/Qt5*.dll ./build/
cp /mingw64/bin/libpng16-16.dll ./build/
cp /mingw64/bin/zlib1.dll ./build/
cp /mingw64/bin/libharfbuzz-0.dll ./build/
cp /mingw64/bin/libfreetype-6.dll ./build/
cp /mingw64/bin/libgraphite2.dll ./build/
cp /mingw64/bin/libdouble-conversion.dll ./build/
cp /mingw64/bin/libicuin*.dll ./build/
cp /mingw64/bin/libicuuc*.dll ./build/
cp /mingw64/bin/libicudt*.dll ./build/
cp /mingw64/bin/libzxing.dll ./build/
cp /mingw64/bin/libprotobuf*.dll ./build/
cp /mingw64/bin/libyaml-cpp.dll ./build/
cp /mingw64/bin/libabsl_*.dll ./build/
cp /mingw64/bin/libutf8_validity.dll ./build/
cp /mingw64/bin/libpcre2-16-0.dll ./build/
cp /mingw64/bin/libzstd.dll ./build/
cp /mingw64/bin/libmd4c.dll ./build/
cp /mingw64/bin/libglib-2.0-0.dll ./build/
cp /mingw64/bin/libbz2-1.dll ./build/
cp /mingw64/bin/libiconv-2.dll ./build/
cp /mingw64/bin/libintl-8.dll ./build/
cp /mingw64/bin/libpcre-1.dll ./build/
cp /mingw64/bin/libpcre2-8-0.dll ./build/
cp /mingw64/bin/libbrotlidec.dll ./build/
cp /mingw64/bin/libbrotlicommon.dll ./build/
cp /mingw64/bin/libexpat-1.dll ./build/
cp /mingw64/bin/libssl-*.dll ./build/
cp /mingw64/bin/libcrypto-*.dll ./build/
```

## 6. 复制 Qt 平台插件
```sh
mkdir -p ./build/platforms
cp /mingw64/share/qt5/plugins/platforms/qwindows.dll ./build/platforms/
```

## 7. 运行
- 进入 build 目录，双击 nekobox.exe 或在 MSYS2 MinGW 64-bit 终端运行：
  ```sh
  cd ./build
  ./nekobox.exe
  ```

---
如遇缺失 DLL 或新依赖，按缺失名称从 /mingw64/bin/ 复制到 build 目录即可。