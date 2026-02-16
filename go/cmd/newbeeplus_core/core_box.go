package main

import (
	"context"
	"net"
	"net/http"

	"github.com/matsuridayo/libneko/neko_common"
	"github.com/matsuridayo/libneko/neko_log"
)

func setupCore() {
	neko_log.SetupLog(50*1024, "./neko.log")

	neko_common.GetCurrentInstance = func() interface{} {
		if port := currentExternalSocksPort(); port > 0 {
			return port
		}
		return nil
	}

	neko_common.DialContext = func(ctx context.Context, specifiedInstance interface{}, network, addr string) (net.Conn, error) {
		return neko_common.DialContextSystem(ctx, network, addr)
	}

	neko_common.DialUDP = func(ctx context.Context, specifiedInstance interface{}) (net.PacketConn, error) {
		return neko_common.DialUDPSystem(ctx)
	}

	neko_common.CreateProxyHttpClient = func(specifiedInstance interface{}) *http.Client {
		if port, ok := specifiedInstance.(int); ok && port > 0 {
			if client, err := createSocks5HttpClient(port); err == nil {
				return client
			}
		}
		if port := currentExternalSocksPort(); port > 0 {
			if client, err := createSocks5HttpClient(port); err == nil {
				return client
			}
		}
		return &http.Client{}
	}
}
