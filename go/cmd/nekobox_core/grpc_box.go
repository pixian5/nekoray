package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grpc_server"
	"grpc_server/gen"

	"github.com/matsuridayo/libneko/neko_common"
	"github.com/matsuridayo/libneko/neko_log"
	"github.com/matsuridayo/libneko/speedtest"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/boxapi"
	boxmain "github.com/sagernet/sing-box/cmd/sing-box"
	"golang.org/x/net/proxy"

	"log"

	"github.com/sagernet/sing-box/option"
)

type server struct {
	grpc_server.BaseServer
}

// createSocks5HttpClient creates an *http.Client that routes traffic through a local SOCKS5 proxy.
func createSocks5HttpClient(port int) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:"+strconv.Itoa(port), nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("SOCKS5 dialer does not support DialContext")
	}
	transport := &http.Transport{
		DialContext:           contextDialer.DialContext,
		TLSHandshakeTimeout:   time.Second * 3,
		ResponseHeaderTimeout: time.Second * 3,
	}
	return &http.Client{Transport: transport}, nil
}

// parseExternalSocksConfig checks if core_config is "EXTERNAL_SOCKS:<port>" and returns the port.
func parseExternalSocksConfig(coreConfig string) (int, bool) {
	const prefix = "EXTERNAL_SOCKS:"
	if strings.HasPrefix(coreConfig, prefix) {
		port, err := strconv.Atoi(coreConfig[len(prefix):])
		if err == nil && port > 0 {
			return port, true
		}
	}
	return 0, false
}

func (s *server) Start(ctx context.Context, in *gen.LoadConfigReq) (out *gen.ErrorResp, _ error) {
	var err error

	defer func() {
		out = &gen.ErrorResp{}
		if err != nil {
			out.Error = err.Error()
			instance = nil
		}
	}()

	if neko_common.Debug {
		log.Println("Start:", in.CoreConfig)
	}

	if instance != nil {
		err = errors.New("instance already started")
		return
	}

	instance, instance_cancel, err = boxmain.Create([]byte(in.CoreConfig))

	if instance != nil {
		// Logger
		instance.SetLogWritter(neko_log.LogWriter)
		// V2ray Service
		if in.StatsOutbounds != nil {
			instance.Router().SetV2RayServer(boxapi.NewSbV2rayServer(option.V2RayStatsServiceOptions{
				Enabled:   true,
				Outbounds: in.StatsOutbounds,
			}))
		}
	}

	return
}

func (s *server) Stop(ctx context.Context, in *gen.EmptyReq) (out *gen.ErrorResp, _ error) {
	var err error

	defer func() {
		out = &gen.ErrorResp{}
		if err != nil {
			out.Error = err.Error()
		}
	}()

	if instance == nil {
		return
	}

	instance_cancel()
	instance.Close()

	instance = nil

	return
}

func (s *server) Test(ctx context.Context, in *gen.TestReq) (out *gen.TestResp, _ error) {
	var err error
	out = &gen.TestResp{Ms: 0}

	defer func() {
		if err != nil {
			out.Error = err.Error()
		}
	}()

	if in.Mode == gen.TestMode_UrlTest {
		// Check if this is an external core test (SOCKS5 proxy)
		if in.Config != nil {
			if port, ok := parseExternalSocksConfig(in.Config.CoreConfig); ok {
				log.Printf("[Test] External core detected, using SOCKS5 on port %d", port)
				httpClient, err2 := createSocks5HttpClient(port)
				if err2 != nil {
					err = err2
					return
				}
				out.Ms, err = speedtest.UrlTest(httpClient, in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
				return
			}
		}

		var i *box.Box
		var cancel context.CancelFunc
		if in.Config != nil {
			// Test instance
			i, cancel, err = boxmain.Create([]byte(in.Config.CoreConfig))
			if i != nil {
				defer i.Close()
				defer cancel()
			}
			if err != nil {
				return
			}
		} else {
			// Test running instance
			i = instance
			if i == nil {
				return
			}
		}
		// Latency
		out.Ms, err = speedtest.UrlTest(boxapi.CreateProxyHttpClient(i), in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
	} else if in.Mode == gen.TestMode_TcpPing {
		out.Ms, err = speedtest.TcpPing(in.Address, in.Timeout)
	} else if in.Mode == gen.TestMode_FullTest {
		// Check if this is an external core full test
		if in.Config != nil {
			if port, ok := parseExternalSocksConfig(in.Config.CoreConfig); ok {
				httpClient, err2 := createSocks5HttpClient(port)
				if err2 != nil {
					err = err2
					return
				}
				return grpc_server.DoFullTestWithHttpClient(ctx, in, httpClient)
			}
		}

		i, cancel, err := boxmain.Create([]byte(in.Config.CoreConfig))
		if i != nil {
			defer i.Close()
			defer cancel()
		}
		if err != nil {
			return
		}
		return grpc_server.DoFullTest(ctx, in, i)
	}

	return
}

func (s *server) QueryStats(ctx context.Context, in *gen.QueryStatsReq) (out *gen.QueryStatsResp, _ error) {
	out = &gen.QueryStatsResp{}

	if instance != nil {
		if ss, ok := instance.Router().V2RayServer().(*boxapi.SbV2rayServer); ok {
			out.Traffic = ss.QueryStats(fmt.Sprintf("outbound>>>%s>>>traffic>>>%s", in.Tag, in.Direct))
		}
	}

	return
}

func (s *server) ListConnections(ctx context.Context, in *gen.EmptyReq) (*gen.ListConnectionsResp, error) {
	out := &gen.ListConnectionsResp{
		// TODO upstream api
	}
	return out, nil
}
