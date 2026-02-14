#include "HTTPRequestHelper.hpp"

#include <QByteArray>
#include <QNetworkProxy>
#include <QEventLoop>
#include <QMetaEnum>
#include <QTimer>

#include "main/NekoGui.hpp"

namespace NekoGui_network {

    NekoHTTPResponse NetworkRequestHelper::HttpGet(const QUrl &url) {
        QNetworkRequest request;
        QNetworkAccessManager accessManager;
        // Default to https if no scheme is provided
        QUrl fixedUrl = url;
        if (fixedUrl.scheme().isEmpty()) {
            fixedUrl.setScheme("https");
        }
        request.setUrl(fixedUrl);
        // Set proxy
        if (NekoGui::dataStore->sub_use_proxy) {
            QNetworkProxy p;
            // Note: sing-box mixed socks5 protocol error
            p.setType(QNetworkProxy::HttpProxy);
            p.setHostName("127.0.0.1");
            p.setPort(NekoGui::dataStore->inbound_socks_port);
            if (NekoGui::dataStore->inbound_auth->NeedAuth()) {
                p.setUser(NekoGui::dataStore->inbound_auth->username);
                p.setPassword(NekoGui::dataStore->inbound_auth->password);
            }
            accessManager.setProxy(p);
            if (NekoGui::dataStore->started_id < 0) {
                return NekoHTTPResponse{QObject::tr("Request with proxy but no profile started.")};
            }
        }
        if (accessManager.proxy().type() == QNetworkProxy::Socks5Proxy) {
            auto cap = accessManager.proxy().capabilities();
            accessManager.proxy().setCapabilities(cap | QNetworkProxy::HostNameLookupCapability);
        }
        // Set attribute
#if (QT_VERSION >= QT_VERSION_CHECK(5, 9, 0))
        request.setAttribute(QNetworkRequest::RedirectPolicyAttribute, QNetworkRequest::NoLessSafeRedirectPolicy);
#endif
        request.setHeader(QNetworkRequest::KnownHeaders::UserAgentHeader, NekoGui::dataStore->GetUserAgent());
        if (NekoGui::dataStore->sub_insecure) {
            QSslConfiguration c;
            c.setPeerVerifyMode(QSslSocket::PeerVerifyMode::VerifyNone);
            request.setSslConfiguration(c);
        }
        //
        auto _reply = accessManager.get(request);
        connect(_reply, &QNetworkReply::sslErrors, _reply, [](const QList<QSslError> &errors) {
            QStringList error_str;
            for (const auto &err: errors) {
                error_str << err.errorString();
            }
            MW_show_log(QStringLiteral("SSL Errors: %1 %2").arg(error_str.join(","), NekoGui::dataStore->sub_insecure ? "(Ignored)" : ""));
        });
        // Wait for response
        auto abortTimer = new QTimer;
        abortTimer->setSingleShot(true);
        abortTimer->setInterval(10000);
        QObject::connect(abortTimer, &QTimer::timeout, _reply, &QNetworkReply::abort);
        abortTimer->start();
        {
            QEventLoop loop;
            QObject::connect(_reply, &QNetworkReply::finished, &loop, &QEventLoop::quit);
            loop.exec();
        }
        if (abortTimer != nullptr) {
            abortTimer->stop();
            abortTimer->deleteLater();
        }
        //
        QString error;
        if (_reply->error() != QNetworkReply::NetworkError::NoError) {
            error = _reply->errorString();

            auto errorEnum = QMetaEnum::fromType<QNetworkReply::NetworkError>();
            const auto *errorName = errorEnum.valueToKey(_reply->error());
            if (errorName != nullptr && *errorName != '\0') {
                error += " (" + QString::fromLatin1(errorName) + ")";
            }

            const auto statusCode = _reply->attribute(QNetworkRequest::HttpStatusCodeAttribute).toInt();
            if (statusCode > 0) {
                error += " [HTTP " + QString::number(statusCode) + "]";
            }
        }

        auto result = NekoHTTPResponse{error, _reply->readAll(), _reply->rawHeaderPairs()};
        _reply->deleteLater();
        return result;
    }

    QString NetworkRequestHelper::GetHeader(const QList<QPair<QByteArray, QByteArray>> &header, const QString &name) {
        for (const auto &p: header) {
            if (QString(p.first).toLower() == name.toLower()) return p.second;
        }
        return "";
    }

} // namespace NekoGui_network
