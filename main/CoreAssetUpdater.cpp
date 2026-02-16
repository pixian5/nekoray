#include "main/CoreAssetUpdater.hpp"

#include <QApplication>
#include <QDateTime>
#include <QDebug>
#include <QDir>
#include <QDirIterator>
#include <QEventLoop>
#include <QFile>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QNetworkProxy>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QProcess>
#include <QRegularExpression>
#include <QTemporaryDir>
#include <QTimer>

#include "main/NekoGui.hpp"

namespace NekoGui_update {
    namespace {
        constexpr auto kPendingSuffix = ".pending-update";

        struct HttpResponse {
            QString error;
            QByteArray data;
            int statusCode = 0;
        };

        struct AssetContext {
            QString displayName;
            QString repoApi;
            QString binaryName;
            QString targetPath;
        };

        inline void append_log(CoreAssetUpdateReport &report, const QString &message, bool interactive) {
            report.lines << message;
            if (interactive || message.startsWith("Error", Qt::CaseInsensitive)) {
                if (MW_show_log) MW_show_log("[CoreUpdate] " + message);
            }
        }

        HttpResponse http_get(const QUrl &url) {
            QNetworkRequest request;
            QNetworkAccessManager accessManager;

            QUrl fixedUrl = url;
            if (fixedUrl.scheme().isEmpty()) fixedUrl.setScheme("https");
            request.setUrl(fixedUrl);
            request.setHeader(QNetworkRequest::UserAgentHeader, NekoGui::dataStore->GetUserAgent());
#if (QT_VERSION >= QT_VERSION_CHECK(5, 9, 0))
            request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::NoLessSafeRedirectPolicy);
#endif

            if (NekoGui::dataStore->sub_use_proxy && NekoGui::dataStore->started_id >= 0) {
                QNetworkProxy p;
                p.setType(QNetworkProxy::HttpProxy);
                p.setHostName("127.0.0.1");
                p.setPort(NekoGui::dataStore->inbound_socks_port);
                if (NekoGui::dataStore->inbound_auth->NeedAuth()) {
                    p.setUser(NekoGui::dataStore->inbound_auth->username);
                    p.setPassword(NekoGui::dataStore->inbound_auth->password);
                }
                accessManager.setProxy(p);
            }

            auto reply = accessManager.get(request);

            auto abortTimer = new QTimer;
            abortTimer->setSingleShot(true);
            abortTimer->setInterval(15000);
            QObject::connect(abortTimer, &QTimer::timeout, reply, &QNetworkReply::abort);
            abortTimer->start();

            {
                QEventLoop loop;
                QObject::connect(reply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
                loop.exec();
            }

            abortTimer->stop();
            abortTimer->deleteLater();

            HttpResponse result;
            result.statusCode = reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();

            if (reply->error() != QNetworkReply::NoError) {
                result.error = reply->errorString();
                if (result.statusCode > 0) {
                    result.error += " [HTTP " + QString::number(result.statusCode) + "]";
                }
            } else {
                result.data = reply->readAll();
            }

            reply->deleteLater();
            return result;
        }

        QString normalize_version(QString version) {
            version = version.trimmed();
            while (version.startsWith("v", Qt::CaseInsensitive)) {
                version = version.mid(1);
            }
            return version.trimmed();
        }

        QList<int> version_numbers(const QString &version) {
            QList<int> out;
            QRegularExpression re(R"((\d+))");
            auto it = re.globalMatch(version);
            while (it.hasNext()) {
                out << it.next().captured(1).toInt();
            }
            return out;
        }

        int compare_version(const QString &left, const QString &right) {
            auto a = version_numbers(normalize_version(left));
            auto b = version_numbers(normalize_version(right));
            auto n = qMax(a.size(), b.size());
            for (int i = 0; i < n; ++i) {
                int av = i < a.size() ? a.at(i) : 0;
                int bv = i < b.size() ? b.at(i) : 0;
                if (av < bv) return -1;
                if (av > bv) return 1;
            }
            return 0;
        }

        QString run_command_output(const QString &program, const QStringList &args) {
            QProcess p;
            p.start(program, args);
            if (!p.waitForStarted(5000)) return {};
            if (!p.waitForFinished(10000)) {
                p.kill();
                p.waitForFinished(1000);
            }
            return QString::fromUtf8(p.readAllStandardOutput() + p.readAllStandardError());
        }

        QString parse_sing_box_version(const QString &output) {
            QRegularExpression re1(R"(sing-box version\s+([^\s]+))", QRegularExpression::CaseInsensitiveOption);
            auto m = re1.match(output);
            if (m.hasMatch()) return normalize_version(m.captured(1));
            QRegularExpression re2(R"(sing-box:\s*([^\s]+))", QRegularExpression::CaseInsensitiveOption);
            m = re2.match(output);
            if (m.hasMatch()) return normalize_version(m.captured(1));
            return {};
        }

        QString parse_xray_version(const QString &output) {
            QRegularExpression re(R"(Xray\s+([^\s]+))", QRegularExpression::CaseInsensitiveOption);
            auto m = re.match(output);
            if (m.hasMatch()) return normalize_version(m.captured(1));
            return {};
        }

        QString current_binary_suffix() {
#ifdef Q_OS_WIN
            return ".exe";
#else
            return {};
#endif
        }

        QString app_binary_path(const QString &name) {
            return QDir::cleanPath(QApplication::applicationDirPath() + "/" + name + current_binary_suffix());
        }

        QString resolve_singbox_target_path() {
            auto coreMap = QString2QJsonObject(NekoGui::dataStore->extraCore->core_map);
            auto configuredPath = coreMap.value("sing-box").toString().trimmed();
            if (!configuredPath.isEmpty()) {
                QFileInfo fi(configuredPath);
                if (fi.isRelative()) {
                    configuredPath = QDir::cleanPath(QApplication::applicationDirPath() + "/" + configuredPath);
                }
                return configuredPath;
            }

            return app_binary_path("sing-box");
        }

        QString resolve_xray_target_path() {
            auto coreMap = QString2QJsonObject(NekoGui::dataStore->extraCore->core_map);
            auto configuredPath = coreMap.value("xray").toString().trimmed();
            if (!configuredPath.isEmpty()) {
                QFileInfo fi(configuredPath);
                if (fi.isRelative()) {
                    configuredPath = QDir::cleanPath(QApplication::applicationDirPath() + "/" + configuredPath);
                }
                return configuredPath;
            }

            auto candidate = QDir::cleanPath(QApplication::applicationDirPath() + "/xray_core/xray" + current_binary_suffix());
            if (QFile::exists(candidate)) return candidate;
            return app_binary_path("xray");
        }

        QJsonObject fetch_latest_release(const QString &apiUrl, QString &error) {
            auto resp = http_get(QUrl(apiUrl));
            if (!resp.error.isEmpty()) {
                error = resp.error;
                return {};
            }
            QJsonParseError pe{};
            auto doc = QJsonDocument::fromJson(resp.data, &pe);
            if (pe.error != QJsonParseError::NoError || !doc.isObject()) {
                error = QObject::tr("Invalid release metadata response.");
                return {};
            }
            return doc.object();
        }

        QString platform_asset_pattern_sing_box() {
#ifdef Q_OS_WIN
            return "windows-amd64";
#elif defined(Q_OS_LINUX)
            return "linux-amd64";
#elif defined(Q_OS_MACOS)
            return "darwin-amd64";
#else
            return {};
#endif
        }

        QString platform_asset_pattern_xray() {
#ifdef Q_OS_WIN
            return "windows-64";
#elif defined(Q_OS_LINUX)
            return "linux-64";
#elif defined(Q_OS_MACOS)
            return "macos-64";
#else
            return {};
#endif
        }

        QString select_asset_url(const QJsonArray &assets, const QString &pattern, const QString &binaryHint) {
            QString zipFallback;
            for (const auto &assetValue: assets) {
                auto asset = assetValue.toObject();
                auto name = asset.value("name").toString();
                auto lower = name.toLower();
                auto url = asset.value("browser_download_url").toString();
                if (url.isEmpty()) continue;
                if (!lower.contains(binaryHint.toLower())) continue;
                if (!pattern.isEmpty() && !lower.contains(pattern.toLower())) continue;
                if (lower.endsWith(".zip")) return url;
                if (lower.endsWith(".tar.gz") || lower.endsWith(".tgz")) zipFallback = url;
            }
            return zipFallback;
        }

        bool write_bytes_to_file(const QByteArray &data, const QString &path, QString &error) {
            QFile out(path);
            if (!out.open(QIODevice::WriteOnly | QIODevice::Truncate)) {
                error = out.errorString();
                return false;
            }
            if (out.write(data) != data.size()) {
                error = out.errorString();
                out.close();
                return false;
            }
            out.close();
            return true;
        }

        bool extract_archive(const QString &archivePath, const QString &destination, QString &error) {
            auto lower = archivePath.toLower();
            QProcess p;
            QString program;
            QStringList args;

            if (lower.endsWith(".zip")) {
#ifdef Q_OS_WIN
                auto quotedArchive = archivePath;
                auto quotedDest = destination;
                quotedArchive.replace("'", "''");
                quotedDest.replace("'", "''");
                program = "powershell";
                args = QStringList{
                    "-NoProfile",
                    "-ExecutionPolicy",
                    "Bypass",
                    "-Command",
                    QString("Expand-Archive -LiteralPath '%1' -DestinationPath '%2' -Force")
                        .arg(quotedArchive, quotedDest),
                };
#else
                program = "unzip";
                args = QStringList{"-o", archivePath, "-d", destination};
#endif
            } else if (lower.endsWith(".tar.gz") || lower.endsWith(".tgz")) {
                program = "tar";
                args = QStringList{"-xzf", archivePath, "-C", destination};
            } else {
                error = QObject::tr("Unsupported archive format.");
                return false;
            }

            p.start(program, args);
            if (!p.waitForStarted(8000)) {
                error = QObject::tr("Failed to start extractor: %1").arg(program);
                return false;
            }
            if (!p.waitForFinished(120000)) {
                p.kill();
                p.waitForFinished(1000);
                error = QObject::tr("Extractor timed out.");
                return false;
            }
            if (p.exitStatus() != QProcess::NormalExit || p.exitCode() != 0) {
                error = QString::fromUtf8(p.readAllStandardOutput() + p.readAllStandardError()).trimmed();
                if (error.isEmpty()) error = QObject::tr("Extractor failed.");
                return false;
            }
            return true;
        }

        QString find_file_recursive(const QString &root, const QString &fileName) {
            QDirIterator it(root, {fileName}, QDir::Files, QDirIterator::Subdirectories);
            if (it.hasNext()) return it.next();
            return {};
        }

        bool queue_pending_file(const QString &sourcePath, const QString &targetPath, QString &error) {
            QDir().mkpath(QFileInfo(targetPath).absolutePath());
            QString pendingPath = targetPath + kPendingSuffix;
            QFile::remove(pendingPath);
            if (!QFile::copy(sourcePath, pendingPath)) {
                error = QObject::tr("Failed to write pending file: %1").arg(pendingPath);
                return false;
            }
            return true;
        }

        bool install_binary_now(const QString &sourcePath, const QString &targetPath, QString &error) {
            QDir().mkpath(QFileInfo(targetPath).absolutePath());

            const QString tempPath = targetPath + ".tmp-update";
            const QString backupPath = targetPath + ".bak";
            QFile::remove(tempPath);
            QFile::remove(backupPath);

            if (!QFile::copy(sourcePath, tempPath)) {
                error = QObject::tr("Failed to stage update file.");
                return false;
            }

            const bool hasTarget = QFile::exists(targetPath);
            if (hasTarget && !QFile::rename(targetPath, backupPath)) {
                QFile::remove(tempPath);
                error = QObject::tr("Target file is in use.");
                return false;
            }

            if (!QFile::rename(tempPath, targetPath)) {
                if (hasTarget && QFile::exists(backupPath)) {
                    QFile::rename(backupPath, targetPath);
                }
                QFile::remove(tempPath);
                error = QObject::tr("Failed to replace target file.");
                return false;
            }

            QFile::remove(backupPath);
            return true;
        }

        QString local_version_of_asset(const AssetContext &ctx) {
            if (!QFile::exists(ctx.targetPath)) return {};
            const auto output = run_command_output(ctx.targetPath, {"version"});
            if (ctx.displayName == "sing-box") return parse_sing_box_version(output);
            if (ctx.displayName == "xray") return parse_xray_version(output);
            return {};
        }

        void handle_asset_update(const AssetContext &ctx, CoreAssetUpdateAction action, bool interactive, CoreAssetUpdateReport &report) {
            QString releaseError;
            auto release = fetch_latest_release(ctx.repoApi, releaseError);
            if (!releaseError.isEmpty()) {
                append_log(report, QString("Error: %1 release check failed: %2").arg(ctx.displayName, releaseError), interactive);
                report.hasError = true;
                return;
            }

            auto remoteVersion = normalize_version(release.value("tag_name").toString());
            auto localVersion = local_version_of_asset(ctx);

            if (!localVersion.isEmpty() && !remoteVersion.isEmpty() && compare_version(localVersion, remoteVersion) >= 0) {
                if (interactive) {
                    append_log(report, QString("%1 is already up to date (%2).").arg(ctx.displayName, localVersion), true);
                }
                return;
            }

            report.hasUpdate = true;
            append_log(report, QString("%1 update found: local=%2 remote=%3")
                                   .arg(ctx.displayName,
                                        localVersion.isEmpty() ? "N/A" : localVersion,
                                        remoteVersion.isEmpty() ? "unknown" : remoteVersion),
                       interactive);

            if (action == CoreAssetUpdateAction::CheckOnly) {
                return;
            }

            auto assets = release.value("assets").toArray();
            QString assetUrl;
            if (ctx.displayName == "sing-box") {
                assetUrl = select_asset_url(assets, platform_asset_pattern_sing_box(), "sing-box");
            } else if (ctx.displayName == "xray") {
                assetUrl = select_asset_url(assets, platform_asset_pattern_xray(), "xray");
            }

            if (assetUrl.isEmpty()) {
                append_log(report, QString("Error: %1 release asset not found for this platform.").arg(ctx.displayName), interactive);
                report.hasError = true;
                return;
            }

            QTemporaryDir tempDir;
            if (!tempDir.isValid()) {
                append_log(report, QString("Error: %1 cannot create temp directory.").arg(ctx.displayName), interactive);
                report.hasError = true;
                return;
            }

            QFileInfo urlInfo(assetUrl);
            auto fileName = urlInfo.fileName();
            if (fileName.isEmpty()) fileName = ctx.displayName + ".download";
            auto archivePath = tempDir.filePath(fileName);

            auto downloadResponse = http_get(QUrl(assetUrl));
            if (!downloadResponse.error.isEmpty()) {
                append_log(report, QString("Error: %1 download failed: %2").arg(ctx.displayName, downloadResponse.error), interactive);
                report.hasError = true;
                return;
            }

            QString fileError;
            if (!write_bytes_to_file(downloadResponse.data, archivePath, fileError)) {
                append_log(report, QString("Error: %1 save failed: %2").arg(ctx.displayName, fileError), interactive);
                report.hasError = true;
                return;
            }

            QString extractedBinaryPath;
            auto lowerName = fileName.toLower();
            if (lowerName.endsWith(".zip") || lowerName.endsWith(".tar.gz") || lowerName.endsWith(".tgz")) {
                QString extractError;
                if (!extract_archive(archivePath, tempDir.path(), extractError)) {
                    append_log(report, QString("Error: %1 extract failed: %2").arg(ctx.displayName, extractError), interactive);
                    report.hasError = true;
                    return;
                }
                extractedBinaryPath = find_file_recursive(tempDir.path(), ctx.binaryName);
            } else {
                extractedBinaryPath = archivePath;
            }

            if (extractedBinaryPath.isEmpty() || !QFile::exists(extractedBinaryPath)) {
                append_log(report, QString("Error: %1 binary not found after extraction.").arg(ctx.displayName), interactive);
                report.hasError = true;
                return;
            }

            if (action == CoreAssetUpdateAction::CheckAndDownload) {
                QString pendingError;
                if (!queue_pending_file(extractedBinaryPath, ctx.targetPath, pendingError)) {
                    append_log(report, QString("Error: %1 download cache failed: %2").arg(ctx.displayName, pendingError), interactive);
                    report.hasError = true;
                    return;
                }
                report.downloaded = true;
                append_log(report, QString("%1 downloaded, pending install: %2").arg(ctx.displayName, ctx.targetPath + kPendingSuffix), interactive);
                return;
            }

            QString installError;
            if (install_binary_now(extractedBinaryPath, ctx.targetPath, installError)) {
                report.downloaded = true;
                report.installed = true;
                append_log(report, QString("%1 updated successfully: %2").arg(ctx.displayName, ctx.targetPath), interactive);
                return;
            }

            QString pendingError;
            if (!queue_pending_file(extractedBinaryPath, ctx.targetPath, pendingError)) {
                append_log(report, QString("Error: %1 install failed (%2), and pending write failed (%3)")
                                       .arg(ctx.displayName, installError, pendingError),
                           interactive);
                report.hasError = true;
                return;
            }

            report.downloaded = true;
            report.needRestart = true;
            append_log(report, QString("%1 is in use. Update is downloaded and will apply after restart.").arg(ctx.displayName), interactive);
        }
    } // namespace

    CoreAssetUpdateReport UpdateCoreAssets(CoreAssetUpdateAction action, bool interactive) {
        CoreAssetUpdateReport report;

        AssetContext singBox{
            "sing-box",
            "https://api.github.com/repos/SagerNet/sing-box/releases/latest",
            "sing-box" + current_binary_suffix(),
            resolve_singbox_target_path(),
        };

        AssetContext xray{
            "xray",
            "https://api.github.com/repos/XTLS/Xray-core/releases/latest",
            "xray" + current_binary_suffix(),
            resolve_xray_target_path(),
        };

        handle_asset_update(singBox, action, interactive, report);
        handle_asset_update(xray, action, interactive, report);

        if (interactive && report.lines.isEmpty()) {
            append_log(report, QObject::tr("sing-box and xray are already up to date."), true);
        }

        return report;
    }

    void ApplyPendingCoreAssetUpdates() {
        auto appDir = QApplication::applicationDirPath();
        QDirIterator it(appDir, QDir::Files, QDirIterator::Subdirectories);
        while (it.hasNext()) {
            auto pendingPath = it.next();
            if (!pendingPath.endsWith(kPendingSuffix)) continue;

            auto targetPath = pendingPath.left(pendingPath.length() - QString(kPendingSuffix).length());
            if (targetPath.isEmpty()) continue;

            auto targetInfo = QFileInfo(targetPath);
            QDir().mkpath(targetInfo.absolutePath());

            QFile::remove(targetPath);
            if (QFile::copy(pendingPath, targetPath)) {
                QFile::remove(pendingPath);
                qDebug() << "[CoreUpdate] Applied pending update:" << targetPath;
            } else {
                qDebug() << "[CoreUpdate] Failed to apply pending update:" << pendingPath;
            }
        }
    }
} // namespace NekoGui_update
