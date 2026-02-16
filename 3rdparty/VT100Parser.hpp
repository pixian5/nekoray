#pragma once

#include <QString>
#include <QRegularExpression>

inline QString cleanVT100String(const QString &in) {
    if (in.isEmpty()) return in;

    QString out = in;

    // CSI: ESC [ ... command
    static const QRegularExpression kAnsiCsi(QStringLiteral("\u001B\\[[0-?]*[ -/]*[@-~]"));
    // OSC: ESC ] ... BEL or ST(ESC \)
    static const QRegularExpression kAnsiOsc(QStringLiteral("\u001B\\][^\\u0007\\u001B]*(?:\\u0007|\\u001B\\\\)"));
    // 2-char escapes: ESC X
    static const QRegularExpression kAnsiEsc2(QStringLiteral("\u001B[@-Z\\\\-_]"));

    out.remove(kAnsiCsi);
    out.remove(kAnsiOsc);
    out.remove(kAnsiEsc2);
    out.remove(QChar(0x1B)); // Any remaining raw ESC bytes

    return out;
}
