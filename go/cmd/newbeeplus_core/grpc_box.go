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
	"github.com/matsuridayo/libneko/speedtest"
	"golang.org/x/net/proxy"
)

type server struct {
	grpc_server.BaseServer
}

type coreRunMode int

const (
	coreModeNone coreRunMode = iota
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

func createRuntimeHttpClient() (*http.Client, error) {
	coreStateMu.Lock()
	runtimeMode := runMode
	runtimeSocksPort := externalSocksPort
	coreStateMu.Unlock()

	if runtimeMode != coreModeExternal || runtimeSocksPort <= 0 {
		return nil, errors.New("instance not started")
	}
	return createSocks5HttpClient(runtimeSocksPort)
}

func createTempExternalHttpClient(coreConfig string, logTag string) (*http.Client, func(), error) {
	testConfig, testPort, prepErr := ensureTestSocksInbound(coreConfig)
	if prepErr != nil {
		return nil, nil, prepErr
	}
	cmd, cancel, configPath, startedPort, startErr := startExternalSingBoxProcess(testConfig, testPort, logTag)
	if startErr != nil {
		return nil, nil, startErr
	}
	httpClient, httpErr := createSocks5HttpClient(startedPort)
	if httpErr != nil {
		stopExternalProcess(cmd, cancel, configPath)
		return nil, nil, httpErr
	}
	cleanup := func() {
		stopExternalProcess(cmd, cancel, configPath)
	}
	return httpClient, cleanup, nil
}

func (s *server) Start(ctx context.Context, in *gen.LoadConfigReq) (out *gen.ErrorResp, _ error) {
	var err error

	defer func() {
		out = &gen.ErrorResp{}
		if err != nil {
			out.Error = err.Error()
		}
	}()

	if neko_common.Debug {
		log.Println("Start:", in.CoreConfig)
	}

	coreStateMu.Lock()
	defer coreStateMu.Unlock()

	if runMode != coreModeNone || externalProcess != nil {
		err = errors.New("instance already started")
		return
	}

	err = startExternalRuntimeLocked(in.CoreConfig)
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
	}

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
		var httpClient *http.Client

		if in.Config != nil {
			if port, ok := parseExternalSocksConfig(in.Config.CoreConfig); ok {
				httpClient, err = createSocks5HttpClient(port)
				if err != nil {
					return
				}
			} else {
				httpClient, cleanup, prepErr := createTempExternalHttpClient(in.Config.CoreConfig, "sing-box-test")
				if prepErr != nil {
					err = prepErr
					return
				}
				defer cleanup()
				out.Ms, err = speedtest.UrlTest(httpClient, in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
				return
			}
		} else {
			httpClient, err = createRuntimeHttpClient()
			if err != nil {
				return
			}
		}

		out.Ms, err = speedtest.UrlTest(httpClient, in.Url, in.Timeout, speedtest.UrlTestStandard_RTT)
		return
	}

	if in.Mode == gen.TestMode_TcpPing {
		out.Ms, err = speedtest.TcpPing(in.Address, in.Timeout)
		return
	}

	if in.Mode == gen.TestMode_FullTest {
		var httpClient *http.Client

		if in.Config != nil {
			if port, ok := parseExternalSocksConfig(in.Config.CoreConfig); ok {
				httpClient, err = createSocks5HttpClient(port)
				if err != nil {
					return
				}
			} else {
				httpClient, cleanup, prepErr := createTempExternalHttpClient(in.Config.CoreConfig, "sing-box-full-test")
				if prepErr != nil {
					err = prepErr
					return
				}
				defer cleanup()
				return grpc_server.DoFullTestWithHttpClient(ctx, in, httpClient)
			}
		} else {
			httpClient, err = createRuntimeHttpClient()
			if err != nil {
				return
			}
		}

		return grpc_server.DoFullTestWithHttpClient(ctx, in, httpClient)
	}

	return
}

func (s *server) QueryStats(ctx context.Context, in *gen.QueryStatsReq) (out *gen.QueryStatsResp, _ error) {
	out = &gen.QueryStatsResp{}
	return out, nil
}

func (s *server) ListConnections(ctx context.Context, in *gen.EmptyReq) (*gen.ListConnectionsResp, error) {
	out := &gen.ListConnectionsResp{
		// TODO upstream api
	}
	return out, nil
}
