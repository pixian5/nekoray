# Windows 编译指南 (MSYS2 MinGW64)

## 前提条件

1. 安装 [MSYS2](https://www.msys2.org/)
2. 安装 [Go](https://go.dev/dl/) >= 1.22，并确保 `go` 在系统 PATH 中

## 一键编译

双击 `build.bat`，或在 MSYS2 MinGW 64-bit 终端中运行：

```sh
bash build.sh
```

脚本会自动完成：安装依赖包、拉取子模块、编译 Go 后端、编译 C++ GUI、打包所有 DLL。

编译产物在 `dist/nekoray/` 目录，直接运行 `nekobox.exe` 即可。

## 仅编译 C++ GUI（跳过 Go）

如果只修改了 C++ 代码，不需要重新编译 Go 后端：

```sh
cd build
cmake -G Ninja -DCMAKE_BUILD_TYPE=Release -DNKR_DISABLE_LIBS=ON ..
ninja
```

## GitHub Actions CI

Push 到 main 或提交 PR 会自动触发 CI 编译（Windows + Linux）。

手动触发 workflow_dispatch 并填写 Release Tag 可发布新版本。
