#pragma once

#include <QStringList>

namespace NekoGui_update {
    enum class CoreAssetUpdateAction : int {
        CheckOnly = 0,
        CheckAndDownload = 1,
        Install = 2,
    };

    struct CoreAssetUpdateReport {
        QStringList lines;
        bool hasUpdate = false;
        bool downloaded = false;
        bool installed = false;
        bool needRestart = false;
        bool hasError = false;
    };

    CoreAssetUpdateReport UpdateCoreAssets(CoreAssetUpdateAction action, bool interactive);

    void ApplyPendingCoreAssetUpdates();
} // namespace NekoGui_update

