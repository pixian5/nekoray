#include "db/ProxyEntity.hpp"
#include "fmt/includes.h"

#include <QFile>
#include <QDir>
#include <QFileInfo>

#define WriteTempFile(fn, data)                                   \
    QDir dir;                                                     \
    if (!dir.exists("temp")) dir.mkdir("temp");                   \
    QFile f(QStringLiteral("temp/") + fn);                        \
    bool ok = f.open(QIODevice::WriteOnly | QIODevice::Truncate); \
    if (ok) {                                                     \
        f.write(data);                                            \
    } else {                                                      \
        result.error = f.errorString();                           \
    }                                                             \
    f.close();                                                    \
    auto TempFile = QFileInfo(f).absoluteFilePath();

namespace NekoGui_fmt {

    // Xray streamSettings builder
    void V2rayStreamSettings::BuildStreamSettingsXray(QJsonObject *outbound) {
        QJsonObject streamSettings;
        streamSettings["network"] = network;

        if (network == "ws") {
            QJsonObject wsSettings;
            if (!path.isEmpty()) wsSettings["path"] = path;
            if (!host.isEmpty()) wsSettings["headers"] = QJsonObject{{"Host", host}};
            streamSettings["wsSettings"] = wsSettings;
        } else if (network == "grpc") {
            QJsonObject grpcSettings;
            if (!path.isEmpty()) grpcSettings["serviceName"] = path;
            streamSettings["grpcSettings"] = grpcSettings;
        } else if (network == "http" || network == "h2") {
            QJsonObject httpSettings;
            if (!path.isEmpty()) httpSettings["path"] = path;
            if (!host.isEmpty()) httpSettings["host"] = QList2QJsonArray(host.split(","));
            streamSettings["httpSettings"] = httpSettings;
            streamSettings["network"] = "http";
        } else if (network == "httpupgrade") {
            QJsonObject httpupgradeSettings;
            if (!path.isEmpty()) httpupgradeSettings["path"] = path;
            if (!host.isEmpty()) httpupgradeSettings["host"] = host;
            streamSettings["httpupgradeSettings"] = httpupgradeSettings;
        } else if (network == "xhttp" || network == "splithttp") {
            QJsonObject xhttpSettings;
            if (!path.isEmpty()) xhttpSettings["path"] = path;
            if (!host.isEmpty()) xhttpSettings["host"] = host;
            if (!xhttp_mode.isEmpty()) xhttpSettings["mode"] = xhttp_mode;
            if (!xhttp_extra.isEmpty()) xhttpSettings["extra"] = xhttp_extra;
            streamSettings["xhttpSettings"] = xhttpSettings;
            streamSettings["network"] = "xhttp";
        } else if (network == "tcp") {
            if (header_type == "http") {
                QJsonObject tcpSettings;
                QJsonObject header;
                header["type"] = "http";
                QJsonObject request;
                if (!path.isEmpty()) request["path"] = QList2QJsonArray(path.split(","));
                if (!host.isEmpty()) request["headers"] = QJsonObject{{"Host", QList2QJsonArray(host.split(","))}};
                header["request"] = request;
                tcpSettings["header"] = header;
                streamSettings["tcpSettings"] = tcpSettings;
            }
        }

        // TLS
        if (!reality_pbk.trimmed().isEmpty()) {
            streamSettings["security"] = "reality";
            QJsonObject realitySettings;
            realitySettings["publicKey"] = reality_pbk;
            realitySettings["shortId"] = reality_sid.split(",")[0];
            if (!sni.trimmed().isEmpty()) realitySettings["serverName"] = sni;
            auto fp = utlsFingerprint.isEmpty() ? "chrome" : utlsFingerprint;
            realitySettings["fingerprint"] = fp;
            if (!reality_spx.isEmpty()) realitySettings["spiderX"] = reality_spx;
            streamSettings["realitySettings"] = realitySettings;
        } else if (security == "tls") {
            streamSettings["security"] = "tls";
            QJsonObject tlsSettings;
            if (!sni.trimmed().isEmpty()) tlsSettings["serverName"] = sni;
            if (allow_insecure || NekoGui::dataStore->skip_cert) tlsSettings["allowInsecure"] = true;
            if (!alpn.trimmed().isEmpty()) tlsSettings["alpn"] = QList2QJsonArray(alpn.split(","));
            if (!utlsFingerprint.isEmpty()) tlsSettings["fingerprint"] = utlsFingerprint;
            if (!certificate.trimmed().isEmpty()) {
                tlsSettings["certificates"] = QJsonArray{QJsonObject{{"certificate", certificate.trimmed()}}};
            }
            streamSettings["tlsSettings"] = tlsSettings;
        }

        outbound->insert("streamSettings", streamSettings);
    }

    // Helper: build full Xray config with SOCKS inbound + single outbound
    static QJsonObject BuildXrayConfig(const QJsonObject &outbound, int socks_port) {
        QJsonObject config;
        config["log"] = QJsonObject{{"loglevel", NekoGui::dataStore->log_level}};

        QJsonArray inbounds;
        inbounds += QJsonObject{
            {"protocol", "socks"},
            {"listen", "127.0.0.1"},
            {"port", socks_port},
            {"settings", QJsonObject{{"udp", true}}},
        };
        config["inbounds"] = inbounds;
        config["outbounds"] = QJsonArray{outbound};

        return config;
    }

    // ============ VMess ============

    CoreObjOutboundBuildResult VMessBean::BuildCoreObjXray() {
        CoreObjOutboundBuildResult result;
        QJsonObject outbound;
        outbound["protocol"] = "vmess";

        QJsonObject user;
        user["id"] = uuid.trimmed();
        user["alterId"] = aid;
        user["security"] = security;

        QJsonObject server;
        server["address"] = serverAddress;
        server["port"] = serverPort;
        server["users"] = QJsonArray{user};

        outbound["settings"] = QJsonObject{{"vnext", QJsonArray{server}}};
        stream->BuildStreamSettingsXray(&outbound);

        result.outbound = outbound;
        return result;
    }

    ExternalBuildResult VMessBean::BuildExternal(int mapping_port, int socks_port, int external_stat) {
        ExternalBuildResult result{NekoGui::dataStore->extraCore->Get("xray")};

        auto coreR = BuildCoreObjXray();
        if (!coreR.error.isEmpty()) {
            result.error = coreR.error;
            return result;
        }

        auto is_direct = external_stat == 2;
        if (!is_direct) {
            // rewrite server address/port to mapping
            auto settings = coreR.outbound["settings"].toObject();
            auto vnext = settings["vnext"].toArray();
            auto server = vnext[0].toObject();
            server["address"] = "127.0.0.1";
            server["port"] = mapping_port;
            vnext[0] = server;
            settings["vnext"] = vnext;
            coreR.outbound["settings"] = settings;
        }

        auto config = BuildXrayConfig(coreR.outbound, socks_port);
        result.config_export = QJsonObject2QString(config, false);
        WriteTempFile("xray_" + GetRandomString(10) + ".json", result.config_export.toUtf8());
        result.arguments = QStringList{"run", "-c", TempFile};

        return result;
    }

    // ============ Trojan / VLESS ============

    CoreObjOutboundBuildResult TrojanVLESSBean::BuildCoreObjXray() {
        CoreObjOutboundBuildResult result;
        QJsonObject outbound;

        if (proxy_type == proxy_VLESS) {
            outbound["protocol"] = "vless";

            QJsonObject user;
            user["id"] = password.trimmed();
            user["encryption"] = "none";
            auto f = flow;
            if (f.right(7) == "-udp443") f.chop(7);
            if (f == "none") f = "";
            if (!f.isEmpty()) user["flow"] = f;

            QJsonObject server;
            server["address"] = serverAddress;
            server["port"] = serverPort;
            server["users"] = QJsonArray{user};

            outbound["settings"] = QJsonObject{{"vnext", QJsonArray{server}}};
        } else {
            outbound["protocol"] = "trojan";

            QJsonObject server;
            server["address"] = serverAddress;
            server["port"] = serverPort;
            server["password"] = password;

            outbound["settings"] = QJsonObject{{"servers", QJsonArray{server}}};
        }

        stream->BuildStreamSettingsXray(&outbound);

        result.outbound = outbound;
        return result;
    }

    ExternalBuildResult TrojanVLESSBean::BuildExternal(int mapping_port, int socks_port, int external_stat) {
        ExternalBuildResult result{NekoGui::dataStore->extraCore->Get("xray")};

        auto coreR = BuildCoreObjXray();
        if (!coreR.error.isEmpty()) {
            result.error = coreR.error;
            return result;
        }

        auto is_direct = external_stat == 2;
        if (!is_direct) {
            auto settings = coreR.outbound["settings"].toObject();
            if (proxy_type == proxy_VLESS) {
                auto vnext = settings["vnext"].toArray();
                auto server = vnext[0].toObject();
                server["address"] = "127.0.0.1";
                server["port"] = mapping_port;
                vnext[0] = server;
                settings["vnext"] = vnext;
            } else {
                auto servers = settings["servers"].toArray();
                auto server = servers[0].toObject();
                server["address"] = "127.0.0.1";
                server["port"] = mapping_port;
                servers[0] = server;
                settings["servers"] = servers;
            }
            coreR.outbound["settings"] = settings;
        }

        auto config = BuildXrayConfig(coreR.outbound, socks_port);
        result.config_export = QJsonObject2QString(config, false);
        WriteTempFile("xray_" + GetRandomString(10) + ".json", result.config_export.toUtf8());
        result.arguments = QStringList{"run", "-c", TempFile};

        return result;
    }

    // ============ Shadowsocks ============

    CoreObjOutboundBuildResult ShadowSocksBean::BuildCoreObjXray() {
        CoreObjOutboundBuildResult result;
        QJsonObject outbound;
        outbound["protocol"] = "shadowsocks";

        QJsonObject server;
        server["address"] = serverAddress;
        server["port"] = serverPort;
        server["method"] = method;
        server["password"] = password;

        outbound["settings"] = QJsonObject{{"servers", QJsonArray{server}}};

        result.outbound = outbound;
        return result;
    }

    ExternalBuildResult ShadowSocksBean::BuildExternal(int mapping_port, int socks_port, int external_stat) {
        ExternalBuildResult result{NekoGui::dataStore->extraCore->Get("xray")};

        auto coreR = BuildCoreObjXray();
        if (!coreR.error.isEmpty()) {
            result.error = coreR.error;
            return result;
        }

        auto is_direct = external_stat == 2;
        if (!is_direct) {
            auto settings = coreR.outbound["settings"].toObject();
            auto servers = settings["servers"].toArray();
            auto server = servers[0].toObject();
            server["address"] = "127.0.0.1";
            server["port"] = mapping_port;
            servers[0] = server;
            settings["servers"] = servers;
            coreR.outbound["settings"] = settings;
        }

        auto config = BuildXrayConfig(coreR.outbound, socks_port);
        result.config_export = QJsonObject2QString(config, false);
        WriteTempFile("xray_" + GetRandomString(10) + ".json", result.config_export.toUtf8());
        result.arguments = QStringList{"run", "-c", TempFile};

        return result;
    }

    // ============ Socks / HTTP ============

    CoreObjOutboundBuildResult SocksHttpBean::BuildCoreObjXray() {
        CoreObjOutboundBuildResult result;
        QJsonObject outbound;
        outbound["protocol"] = socks_http_type == type_HTTP ? "http" : "socks";

        QJsonObject server;
        server["address"] = serverAddress;
        server["port"] = serverPort;
        if (!username.isEmpty() && !password.isEmpty()) {
            QJsonObject user;
            user["user"] = username;
            user["pass"] = password;
            server["users"] = QJsonArray{user};
        }

        outbound["settings"] = QJsonObject{{"servers", QJsonArray{server}}};
        stream->BuildStreamSettingsXray(&outbound);

        result.outbound = outbound;
        return result;
    }

    ExternalBuildResult SocksHttpBean::BuildExternal(int mapping_port, int socks_port, int external_stat) {
        ExternalBuildResult result{NekoGui::dataStore->extraCore->Get("xray")};

        auto coreR = BuildCoreObjXray();
        if (!coreR.error.isEmpty()) {
            result.error = coreR.error;
            return result;
        }

        auto is_direct = external_stat == 2;
        if (!is_direct) {
            auto settings = coreR.outbound["settings"].toObject();
            auto servers = settings["servers"].toArray();
            auto server = servers[0].toObject();
            server["address"] = "127.0.0.1";
            server["port"] = mapping_port;
            servers[0] = server;
            settings["servers"] = servers;
            coreR.outbound["settings"] = settings;
        }

        auto config = BuildXrayConfig(coreR.outbound, socks_port);
        result.config_export = QJsonObject2QString(config, false);
        WriteTempFile("xray_" + GetRandomString(10) + ".json", result.config_export.toUtf8());
        result.arguments = QStringList{"run", "-c", TempFile};

        return result;
    }

} // namespace NekoGui_fmt
