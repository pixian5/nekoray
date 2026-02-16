package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"grpc_server"
	"grpc_server/gen"

	"github.com/matsuridayo/libneko/neko_common"
	"github.com/matsuridayo/libneko/neko_log"
	"github.com/matsuridayo/libneko/speedtest"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/boxapi"
	boxmain "github.com/sagernet/sing-box/cmd/sing-box"
	"github.com/sagernet/sing-box/option"
	"golang.org/x/net/proxy"
)

type server struct {
	grpc_server.BaseServer
}

type coreRunMode int

const (
	coreModeNone coreRunMode = iota
	coreModeEmbedded
	coreModeExternal
)

var (
	coreStateMu sync.Mutex
	runMode     coreRunMode = coreModeNone

	externalProcess       *exec.Cmd
	externalProcessCancel context.CancelFunc
	externalConfigPath    string
	externalSocksPort     int
)

func currentExternalSocksPort() int {
	coreStateMu.Lock()
	defer coreStateMu.Unlock()
	if runMode == coreModeExternal {
		return externalSocksPort
	}
	return 0
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

func parseSocksPortFromConfig(coreConfig string) int {
	if coreConfig == "" {
		return 0
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(coreConfig), &root); err != nil {
		return 0
	}
	rawInbounds, ok := root["inbounds"]
	if !ok {
		return 0
	}
	inbounds, ok := rawInbounds.([]interface{})
	if !ok {
		return 0
	}
	for _, item := range inbounds {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := obj["type"].(string)
		if t != "mixed" && t != "socks" && t != "http" {
			continue
		}
		portValue, ok := obj["listen_port"].(float64)
		if !ok {
			continue
		}
		port := int(portValue)
		if port > 0 {
			return port
		}
	}
	return 0
}

func allocateTCPPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, errors.New("failed to allocate tcp port")
	}
	return addr.Port, nil
}

func ensureTestSocksInbound(coreConfig string) (string, int, error) {
	if coreConfig == "" {
		return "", 0, errors.New("empty core config")
	}

	var root map[string]interface{}
	if err := json.Unmarshal([]byte(coreConfig), &root); err != nil {
		return "", 0, err
	}

	if existing := parseSocksPortFromConfig(coreConfig); existing > 0 {
		return coreConfig, existing, nil
	}

	port, err := allocateTCPPort()
	if err != nil {
		return "", 0, err
	}

	inbound := map[string]interface{}{
		"tag":         "mixed-test",
		"type":        "mixed",
		"listen":      "127.0.0.1",
		"listen_port": port,
	}

	rawInbounds, ok := root["inbounds"]
	if !ok {
		root["inbounds"] = []interface{}{inbound}
	} else if inbounds, ok := rawInbounds.([]interface{}); ok {
		root["inbounds"] = append(inbounds, inbound)
	} else {
		root["inbounds"] = []interface{}{inbound}
	}

	b, err := json.Marshal(root)
	if err != nil {
		return "", 0, err
	}
	return string(b), port, nil
}

func writeTempConfigFile(coreConfig string) (string, error) {
	dir := filepath.Clean("./temp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "sing-box-external-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(coreConfig); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func waitLocalPortReady(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(120 * time.Millisecond)
	}
	return false
}

func forwardProcessLog(prefix string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			log.Printf("[%s] %s", prefix, line)
		}
	}
}

func startExternalSingBoxProcess(coreConfig string, socksPort int, logTag string) (*exec.Cmd, context.CancelFunc, string, int, error) {
	if socksPort <= 0 {
		socksPort = parseSocksPortFromConfig(coreConfig)
	}
	if socksPort <= 0 {
		return nil, nil, "", 0, errors.New("socks/mixed inbound port not found in sing-box config")
	}

	binary, err := resolveSingBoxExecutablePath()
	if err != nil {
		return nil, nil, "", 0, err
	}

	configPath, err := writeTempConfigFile(coreConfig)
	if err != nil {
		return nil, nil, "", 0, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "run", "-c", configPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = os.Remove(configPath)
		return nil, nil, "", 0, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		_ = os.Remove(configPath)
		return nil, nil, "", 0, err
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.Remove(configPath)
		return nil, nil, "", 0, err
	}

	go forwardProcessLog(logTag, stdout)
	go forwardProcessLog(logTag, stderr)

	if !waitLocalPortReady(socksPort, 8*time.Second) {
		cancel()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = os.Remove(configPath)
		return nil, nil, "", 0, errors.New("external sing-box socks port not ready")
	}

	return cmd, cancel, configPath, socksPort, nil
}

func stopExternalProcess(cmd *exec.Cmd, cancel context.CancelFunc, configPath string) {
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if configPath != "" {
		_ = os.Remove(configPath)
	}
}

func stopExternalRuntimeLocked() {
	cmd := externalProcess
	cancel := externalProcessCancel
	configPath := externalConfigPath

	externalProcess = nil
	externalProcessCancel = nil
	externalConfigPath = ""
	externalSocksPort = 0
	if runMode == coreModeExternal {
		runMode = coreModeNone
	}

	stopExternalProcess(cmd, cancel, configPath)
}

func startExternalRuntimeLocked(coreConfig string) error {
	socksPort := parseSocksPortFromConfig(coreConfig)
	if socksPort <= 0 {
		return errors.New("runtime config missing socks/mixed inbound port")
	}

	cmd, cancel, configPath, runtimePort, err := startExternalSingBoxProcess(coreConfig, socksPort, "sing-box-ext")
	if err != nil {
		return err
	}

	externalProcess = cmd
	externalProcessCancel = cancel
	externalConfigPath = configPath
	externalSocksPort = runtimePort
	runMode = coreModeExternal

	go func(proc *exec.Cmd, procConfigPath string) {
		err := proc.Wait()
		log.Printf("[sing-box-ext] process exited: %v", err)
		_ = os.Remove(procConfigPath)
		coreStateMu.Lock()
		if externalProcess == proc {
			externalProcess = nil
			externalProcessCancel = nil
			externalConfigPath = ""
			externalSocksPort = 0
			if runMode == coreModeExternal {
				runMode = coreModeNone
			}
		}
		coreStateMu.Unlock()
	}(cmd, configPath)

	return nil
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

	coreStateMu.Lock()
	defer coreStateMu.Unlock()

	if runMode != coreModeNone || instance != nil || externalProcess != nil {
		err = errors.New("instance already started")
		return
	}

	if singBoxExternalPreferred() {
		if startErr := startExternalRuntimeLocked(in.CoreConfig); startErr == nil {
			return
		} else if singBoxExternalRequired() {
			err = startErr
			return
		} else {
			log.Printf("[sing-box-ext] start failed, fallback to embedded: %v", startErr)
		}
	}

	instance, instance_cancel, err = boxmain.Create([]byte(in.CoreConfig))
	if err != nil {
		return
	}

	runMode = coreModeEmbedded

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

	coreStateMu.Lock()
	defer coreStateMu.Unlock()

	if runMode == coreModeExternal {
		stopExternalRuntimeLocked()
		return
	}

	if instance == nil {
		runMode = coreModeNone
		return
	}

	instance_cancel()
	instance.Close()
	instance = nil
	runMode = coreModeNone

	return
}

func (s *server) Test(ctx context.Context, in *gen.TestReq) (out *gen.TestResp, _ error) {
	var err error
	out = &gen.TestResp{Ms: 0}

	in.Url = strings.TrimSpace(in.Url)
	if in.Url == "" {
		in.Url = "https://www.youtube.com/generate_204"
	}
	in.FullSpeedUrl = strings.TrimSpace(in.FullSpeedUrl)
	if in.FullSpeedUrl == "" {
		in.FullSpeedUrl = "http://cachefly.cachefly.net/10mb.test"
	}

	defer func() {
		if err != nil {
			out.Error = err.Error()
		}
	}()

	if in.Mode == gen.TestMode_UrlTest {
		// External core shortcut: use provided SOCKS5 port directly.
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

		// Preferred: external sing-box temp instance for per-node tests.
		if in.Config != nil && singBoxExternalPreferred() {
			testConfig, testPort, prepErr := ensureTestSocksInbound(in.Config.CoreConfig)
			if prepErr == nil {
				cmd, cancel, configPath, startedPort, startErr := startExternalSingBoxProcess(testConfig, testPort, "sing-box-test")
				if startErr == nil {
					defer stopExternalProcess(cmd, cancel, configPath)
					httpClient, httpErr := createSocks5HttpClient(startedPort)
					if httpErr != nil {
						err = httpErr
						return
					}
					out.Ms, err = speedtest.UrlTest(httpClient, in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
					return
				}
				prepErr = startErr
			}
			if singBoxExternalRequired() {
				err = prepErr
				return
			}
			log.Printf("[Test] external sing-box test failed, fallback to embedded: %v", prepErr)
		}

		// Running instance url-test: external runtime uses its local socks port.
		coreStateMu.Lock()
		runtimeMode := runMode
		runtimeSocksPort := externalSocksPort
		runtimeInstance := instance
		coreStateMu.Unlock()

		if in.Config == nil && runtimeMode == coreModeExternal {
			if runtimeSocksPort <= 0 {
				err = errors.New("external runtime socks port unavailable")
				return
			}
			httpClient, err2 := createSocks5HttpClient(runtimeSocksPort)
			if err2 != nil {
				err = err2
				return
			}
			out.Ms, err = speedtest.UrlTest(httpClient, in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
			return
		}

		// Fallback: embedded instance (running or temp).
		var i *box.Box
		var cancel context.CancelFunc
		if in.Config != nil {
			i, cancel, err = boxmain.Create([]byte(in.Config.CoreConfig))
			if i != nil {
				defer i.Close()
				defer cancel()
			}
			if err != nil {
				return
			}
		} else {
			i = runtimeInstance
			if i == nil {
				err = errors.New("instance not started")
				return
			}
		}
		out.Ms, err = speedtest.UrlTest(boxapi.CreateProxyHttpClient(i), in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
		return
	}

	if in.Mode == gen.TestMode_TcpPing {
		out.Ms, err = speedtest.TcpPing(in.Address, in.Timeout)
		return
	}

	if in.Mode == gen.TestMode_FullTest {
		// External core shortcut: use provided SOCKS5 port directly.
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

		// Preferred: external sing-box temp instance for per-node full test.
		if in.Config != nil && singBoxExternalPreferred() {
			testConfig, testPort, prepErr := ensureTestSocksInbound(in.Config.CoreConfig)
			if prepErr == nil {
				cmd, cancel, configPath, startedPort, startErr := startExternalSingBoxProcess(testConfig, testPort, "sing-box-full-test")
				if startErr == nil {
					defer stopExternalProcess(cmd, cancel, configPath)
					httpClient, httpErr := createSocks5HttpClient(startedPort)
					if httpErr != nil {
						err = httpErr
						return
					}
					return grpc_server.DoFullTestWithHttpClient(ctx, in, httpClient)
				}
				prepErr = startErr
			}
			if singBoxExternalRequired() {
				err = prepErr
				return
			}
			log.Printf("[Test] external sing-box full-test failed, fallback to embedded: %v", prepErr)
		}

		coreStateMu.Lock()
		runtimeMode := runMode
		runtimeSocksPort := externalSocksPort
		runtimeInstance := instance
		coreStateMu.Unlock()

		if in.Config == nil && runtimeMode == coreModeExternal {
			if runtimeSocksPort <= 0 {
				err = errors.New("external runtime socks port unavailable")
				return
			}
			httpClient, err2 := createSocks5HttpClient(runtimeSocksPort)
			if err2 != nil {
				err = err2
				return
			}
			return grpc_server.DoFullTestWithHttpClient(ctx, in, httpClient)
		}

		var (
			i         *box.Box
			cancel    context.CancelFunc
			createErr error
		)
		if in.Config == nil {
			i = runtimeInstance
		} else {
			i, cancel, createErr = boxmain.Create([]byte(in.Config.CoreConfig))
		}
		if i != nil && cancel != nil {
			defer i.Close()
			defer cancel()
		}
		if createErr != nil {
			err = createErr
			return
		}
		if i == nil {
			err = errors.New("instance not started")
			return
		}
		return grpc_server.DoFullTest(ctx, in, i)
	}

	return
}

func (s *server) QueryStats(ctx context.Context, in *gen.QueryStatsReq) (out *gen.QueryStatsResp, _ error) {
	out = &gen.QueryStatsResp{}

	coreStateMu.Lock()
	defer coreStateMu.Unlock()
	if runMode != coreModeEmbedded || instance == nil {
		return out, nil
	}

	if ss, ok := instance.Router().V2RayServer().(*boxapi.SbV2rayServer); ok {
		out.Traffic = ss.QueryStats(fmt.Sprintf("outbound>>>%s>>>traffic>>>%s", in.Tag, in.Direct))
	}

	return
}

func (s *server) ListConnections(ctx context.Context, in *gen.EmptyReq) (*gen.ListConnectionsResp, error) {
	out := &gen.ListConnectionsResp{
		// TODO upstream api
	}
	return out, nil
}
