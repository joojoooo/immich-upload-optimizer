package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
)

var version = "dev"
var commit = "none"
var date = "unknown"

var remote *url.URL
var proxyUrl *url.URL

var maxConcurrentRequests = 10
var semaphore = make(chan struct{}, maxConcurrentRequests)

var showVersion bool
var upstreamURL string
var listenAddr string
var configFile string
var filterPath string
var filterFormKey string
var downloadJpgFromJxl bool

var config *Config

func init() {
	viper.SetEnvPrefix("iuo")
	viper.AutomaticEnv()
	viper.BindEnv("upstream")
	viper.BindEnv("listen")
	viper.BindEnv("tasks_file")
	viper.BindEnv("filter_path")
	viper.BindEnv("filter_form_key")
	viper.BindEnv("download_jpg_from_jxl")

	viper.SetDefault("upstream", "")
	viper.SetDefault("listen", ":2284")
	viper.SetDefault("tasks_file", "config/lossless.yaml")
	viper.SetDefault("filter_path", "/api/assets")
	viper.SetDefault("filter_form_key", "assetData")
	viper.SetDefault("download_jpg_from_jxl", false)

	flag.BoolVar(&showVersion, "version", false, "Show the current version")
	flag.StringVar(&upstreamURL, "upstream", viper.GetString("upstream"), "Upstream URL. Example: http://immich-server:2283")
	flag.StringVar(&listenAddr, "listen", viper.GetString("listen"), "Listening address")
	flag.StringVar(&configFile, "tasks_file", viper.GetString("tasks_file"), "Path to the configuration file")
	flag.StringVar(&filterPath, "filter_path", viper.GetString("filter_path"), "Only convert files uploaded to specific path. Advanced, leave default for immich")
	flag.StringVar(&filterFormKey, "filter_form_key", viper.GetString("filter_form_key"), "Only convert files uploaded with specific form key. Advanced, leave default for immich")
	flag.BoolVar(&downloadJpgFromJxl, "download_jpg_from_jxl", viper.GetBool("download_jpg_from_jxl"), "Converts JXL images to JPG on download for wider compatibility")
	flag.Parse()

	if showVersion {
		fmt.Println(printVersion())
		os.Exit(0)
	}

	validateInput()

	proxyUrl, _ = url.Parse("http://localhost:8080")
}

var baseLogger *log.Logger
var proxy *httputil.ReverseProxy

var DevMITMproxy = version == "dev"

func main() {
	baseLogger = log.New(os.Stdout, "", log.Ldate|log.Ltime)
	log.Printf("Starting %s on %s...", printVersion(), listenAddr)
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir != "" {
		info, err := os.Stat(tmpDir)
		if err == nil && info.IsDir() {
			log.Printf("tmp directory: %s", tmpDir)
			_ = removeAllContents(tmpDir)
		} else {
			panic("TMPDIR must be a directory")
		}
	} else {
		log.Printf("no tmp directory set, uploaded files will be saved on disk multiple times, this can shorten your disk lifespan !")
	}
	// Proxy
	proxy = httputil.NewSingleHostReverseProxy(remote)
	if DevMITMproxy {
		proxy.Transport = http.DefaultTransport
		proxy.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyUrl)
	}
	server := &http.Server{Addr: listenAddr, Handler: http.HandlerFunc(handleRequest)}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting immich-upload-optimizer: %v", err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	var err error
	requestLogger := newCustomLogger(baseLogger, fmt.Sprintf("%s: ", strings.Split(r.RemoteAddr, ":")[0]))
	if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
		requestLogger.SetErrPrefix("websocket")
		requestLogger.Printf("websocket upgrade")
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}
		var cliConn, srvConn *websocket.Conn
		if cliConn, err = upgrader.Upgrade(w, r, nil); requestLogger.Error(err, "upgrade") {
			return
		}
		defer cliConn.Close()
		header := r.Header.Clone()
		header.Del("Upgrade")
		header.Del("Connection")
		header.Del("Sec-Websocket-Key")
		header.Del("Sec-Websocket-Version")
		header.Del("Sec-Websocket-Extensions")
		header.Del("Sec-Websocket-Protocol")
		if srvConn, _, err = websocket.DefaultDialer.Dial("ws"+upstreamURL[strings.Index(upstreamURL, ":"):]+r.URL.String(), header); requestLogger.Error(err, "dial") {
			return
		}
		defer srvConn.Close()
		handleWebSocketConn(cliConn, srvConn, requestLogger)
		return
	}
	requestLogger.Printf("proxy path: %s", r.URL.String())
	switch r.Method {
	case "POST":
		requestLogger.SetErrPrefix("upload error")
		var match, matchDelta bool
		if match, err = path.Match(filterPath, r.URL.Path); requestLogger.Error(err, "invalid filter_path") {
			break
		}
		if match && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			err = newJob(r, w, requestLogger)
			requestLogger.Error(err, "")
			return
		}
		// Sync: replace checksum
		requestLogger.SetErrPrefix("sync error")
		if match, err = path.Match("/api/sync/full-sync", r.URL.Path); requestLogger.Error(err, "invalid filter_path") {
			break
		}
		if matchDelta, err = path.Match("/api/sync/delta-sync", r.URL.Path); requestLogger.Error(err, "invalid filter_path") {
			break
		}
		if !match && !matchDelta {
			break
		}
		var req *http.Request
		var resp *http.Response
		if req, err = http.NewRequest("POST", upstreamURL+r.URL.String(), nil); requestLogger.Error(err, "new POST") {
			break
		}
		req.Header = r.Header
		req.Body = r.Body
		if resp, err = getHTTPclient().Do(req); requestLogger.Error(err, "getHTTPclient.Do") {
			break
		}
		defer resp.Body.Close()
		var jsonBuf []byte
		if jsonBuf, err = io.ReadAll(resp.Body); requestLogger.Error(err, "resp read") {
			break
		}
		if match {
			var assets []Asset
			if err = json.Unmarshal(jsonBuf, &assets); requestLogger.Error(err, "json unmarshal") {
				break
			}
			for _, asset := range assets {
				asset.ToOriginalAsset()
			}
			if jsonBuf, err = json.Marshal(assets); requestLogger.Error(err, "json marshal") {
				break
			}
		} else {
			var deltaMap map[string]any
			if err = json.Unmarshal(jsonBuf, &deltaMap); requestLogger.Error(err, "json unmarshal") {
				break
			}
			for key, up := range deltaMap {
				if key != "upserted" {
					continue
				}
				if upserted, ok := up.([]any); ok {
					for _, a := range upserted {
						if asset, ok := a.(map[string]any); ok {
							Asset(asset).ToOriginalAsset()
						}
					}
				}
				break
			}
			if jsonBuf, err = json.Marshal(deltaMap); requestLogger.Error(err, "json marshal") {
				break
			}
		}
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		if _, err = w.Write(jsonBuf); requestLogger.Error(err, "unable to forward response to client") {
			break
		}
		return
		// Albums:
	case "GET":
		// JXL download and JPG conversion
		if !downloadJpgFromJxl {
			break
		}
		match, _ := path.Match(filterPath+"/*/original", r.URL.Path)
		if match {
			//TODO: get asset info from immich and only download if JXL extension
			requestLogger.Printf("converting: %s", r.URL)
			requestLogger.SetErrPrefix("conversion error")

			var req *http.Request
			var resp *http.Response
			var blob *os.File
			if req, err = http.NewRequest("GET", upstreamURL+r.URL.String(), nil); requestLogger.Error(err, "new GET") {
				break
			}
			req.Header = r.Header
			if resp, err = getHTTPclient().Do(req); requestLogger.Error(err, "getHTTPclient.Do") {
				break
			}
			if blob, err = os.CreateTemp("", "blob-*"); requestLogger.Error(err, "blob create") {
				break
			}
			cleanupBlob := func() { blob.Close(); _ = os.Remove(blob.Name()) }
			defer cleanupBlob()
			if _, err = io.Copy(blob, resp.Body); requestLogger.Error(err, "blob copy") {
				break
			}
			resp.Body.Close()
			if _, err = blob.Seek(0, io.SeekStart); requestLogger.Error(err, "blob seek") {
				break
			}
			signature := make([]byte, 12)
			if _, err = blob.Read(signature); requestLogger.Error(err, "blob read") {
				break
			}
			if bytes.Equal(signature, []byte{0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A}) {
				var output []byte
				var open *os.File
				if output, err = exec.Command("djxl", blob.Name(), blob.Name()+".jpg").CombinedOutput(); requestLogger.Error(err, "djxl") {
					break
				}
				requestLogger.Printf("conversion complete: %s", strings.ReplaceAll(string(output), "\n", " - "))
				cleanupBlob()
				if open, err = os.Open(blob.Name() + ".jpg"); requestLogger.Error(err, "open jpg") {
					break
				}
				defer func() { open.Close(); _ = os.Remove(open.Name()) }()
				if _, err = io.Copy(w, open); requestLogger.Error(err, "write resp") {
					break
				}
				return
			}
		}
	}

	r.Host = remote.Host
	proxy.ServeHTTP(w, r)
}
