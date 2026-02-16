#!/bin/bash
# 完整构建脚本 - 添加 git 到 PATH
export PATH="/mingw64/bin:/usr/bin:$PATH"

# 确保 git 可用 (MSYS2 的 git 在 /usr/bin)
if ! command -v git &>/dev/null; then
    pacman -S --noconfirm --needed git
fi

cd /d/00code/nekoray
bash build.sh 2>&1
