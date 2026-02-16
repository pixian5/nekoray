#!/bin/bash
# ============================================================
# 自动收集 MinGW64 DLL 依赖到目标目录
# 用法: bash libs/collect_dlls_mingw.sh <目标目录>
#
# 使用 ntldd 递归分析 EXE/DLL 的依赖关系，自动复制所有
# 来自 mingw64 的 DLL 到目标目录，无需手动维护 DLL 列表。
# ============================================================
set -e

DEST="${1:?用法: $0 <目标目录>}"

if [ ! -d "$DEST" ]; then
    echo "错误: 目标目录不存在: $DEST"
    exit 1
fi

# 确保 ntldd 可用
if ! command -v ntldd &>/dev/null; then
    echo "正在安装 ntldd ..."
    pacman -S --noconfirm --needed mingw-w64-x86_64-ntldd
fi

MINGW_BIN="/mingw64/bin"

# 收集目标目录下所有 EXE 和 DLL (含子目录) 的依赖
collect_deps() {
    local target_dir="$1"
    # 扫描所有 exe 和 dll
    find "$target_dir" \( -iname '*.exe' -o -iname '*.dll' \) -print0 2>/dev/null \
        | xargs -0 -r -n1 ntldd -R 2>/dev/null || true
}

# 从 ntldd 输出中提取 mingw64 DLL 名称
# ntldd 输出格式: "  libfoo.dll => C:\msys64\mingw64\bin\libfoo.dll (0x...)"
# 注意: ntldd 输出 Windows 反斜杠路径，basename 无法正确处理，
# 所以直接用 awk 取第一列 (即 DLL 文件名)。
extract_mingw_dlls() {
    grep -i 'mingw64[/\\]bin[/\\]' \
        | awk '{print $1}' \
        | sort -u
}

copy_round() {
    local round_name="$1"
    local deps
    deps=$(collect_deps "$DEST")

    local dll_names
    dll_names=$(echo "$deps" | extract_mingw_dlls)

    local copied=0
    local skipped=0

    while IFS= read -r dll_name; do
        [ -z "$dll_name" ] && continue

        # 跳过已存在的
        if [ -f "$DEST/$dll_name" ]; then
            ((skipped++)) || true
            continue
        fi

        if [ -f "$MINGW_BIN/$dll_name" ]; then
            cp "$MINGW_BIN/$dll_name" "$DEST/"
            ((copied++)) || true
        fi
    done <<< "$dll_names"

    echo "  [$round_name] 复制了 $copied 个 DLL, 跳过 $skipped 个已存在的"
    echo "$copied"
}

echo "=== 分析 DLL 依赖 ==="

# 第一轮
copied=$(copy_round "第一轮")
copied=${copied##*$'\n'}  # 取最后一行 (数字)

# 第二轮: 新复制的 DLL 可能引入新依赖
if [ "$copied" -gt 0 ]; then
    copied2=$(copy_round "第二轮")
    copied2=${copied2##*$'\n'}

    # 第三轮: 极端情况下可能还有更深层依赖
    if [ "$copied2" -gt 0 ]; then
        copy_round "第三轮" > /dev/null
    fi
fi

# 运行时加载的 DLL (ntldd 无法检测到)
# OpenSSL: Qt 的 SSL 后端在运行时动态加载
RUNTIME_DLLS=(
    "libssl-3-x64.dll"
    "libcrypto-3-x64.dll"
)

for dll_name in "${RUNTIME_DLLS[@]}"; do
    if [ ! -f "$DEST/$dll_name" ] && [ -f "$MINGW_BIN/$dll_name" ]; then
        cp "$MINGW_BIN/$dll_name" "$DEST/"
        echo "  [运行时] 复制 $dll_name"
    fi
done

# 验证: 检查是否还有缺失的非系统 DLL
echo "=== 验证 DLL 完整性 ==="
missing=$(collect_deps "$DEST" \
    | grep 'not found' \
    | grep -iv 'ext-ms-' \
    | grep -iv 'api-ms-' \
    | grep -iv 'PdmUtilities' \
    | grep -iv 'HvsiFileTrust' \
    | sed 's/^[[:space:]]*//' \
    | sort -u)

if [ -n "$missing" ]; then
    echo "警告: 仍有未解析的 DLL:"
    echo "$missing"
    echo ""
    echo "这些 DLL 可能需要手动处理。"
else
    echo "  所有 DLL 依赖已满足 ✓"
fi
