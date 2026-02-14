@echo off
REM nekoray 一键编译 - Windows 启动器
REM 前提: 已安装 MSYS2 (https://www.msys2.org/) 和 Go (https://go.dev/dl/)
REM 双击此文件即可开始编译

if not exist "C:\msys64\msys2_shell.cmd" (
    echo 错误: 未找到 MSYS2，请先安装 MSYS2
    echo 下载: https://www.msys2.org/
    pause
    exit /b 1
)

C:\msys64\msys2_shell.cmd -mingw64 -defterm -no-start -here -c "bash build.sh"
pause
