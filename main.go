package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

// goreleaser auto updated vars
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
var checksumsFile string
var downloadJpgFromJxl bool

var config *Config

func init() {
	viper.SetEnvPrefix("iuo")
	viper.AutomaticEnv()
	viper.BindEnv("upstream")
	viper.BindEnv("listen")
	viper.BindEnv("tasks_file")
	viper.BindEnv("download_jpg_from_jxl")

	viper.SetDefault("upstream", "")
	viper.SetDefault("listen", ":2284")
	viper.SetDefault("tasks_file", "config/lossy_avif.yaml")
	viper.SetDefault("checksums_file", "checksums.csv")
	viper.SetDefault("download_jpg_from_jxl", false)

	flag.BoolVar(&showVersion, "version", false, "Show the current version")
	flag.StringVar(&upstreamURL, "upstream", viper.GetString("upstream"), "Upstream URL. Example: http://immich-server:2283")
	flag.StringVar(&listenAddr, "listen", viper.GetString("listen"), "Listening address")
	flag.StringVar(&configFile, "tasks_file", viper.GetString("tasks_file"), "Path to the configuration file")
	flag.StringVar(&checksumsFile, "checksums_file", viper.GetString("checksums_file"), "Path to the checksums file")
	flag.BoolVar(&downloadJpgFromJxl, "download_jpg_from_jxl", viper.GetBool("download_jpg_from_jxl"), "Converts JXL images to JPG on download for wider compatibility")
	flag.Parse()

	if showVersion {
		fmt.Println(printVersion())
		os.Exit(0)
	}

	validateInput()

	proxyUrl, _ = url.Parse("http://localhost:8080")
	initChecksums()
}

var baseLogger *log.Logger
var proxy *httputil.ReverseProxy

// DevMITMproxy Used for development, version gets automatically replaced by goreleaser, making this false
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
	logger := newCustomLogger(baseLogger, fmt.Sprintf("%s: ", strings.Split(r.RemoteAddr, ":")[0]))
	if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
		upgradeWebSocketRequest(w, r, logger)
		return
	}
	defer func() {
		// Only print URL if the request was handled by IUO
		if logger.HasErrPrefix() {
			logger.Printf("request URL: %s", r.URL.String())
		}
	}()
	switch {
	case isAssetsUpload(r):
		err = newJob(r, w, logger)
		logger.SetErrPrefix("upload")
		logger.Error(err, "")
		return
	case downloadJpgFromJxl && isOriginalDownloadPath(r):
		if err = downloadAndConvertImage(w, r, logger); err == nil {
			return
		}
	default:
		if replacer := getChecksumReplacer(r); replacer != nil {
			logger.SetErrPrefix(strings.TrimPrefix(reflect.TypeOf(replacer).String(), "main."))
			err = replacer.Replace(w, r, logger)
			if err == nil {
				return
			}
		}
	}
	r.Host = remote.Host
	proxy.ServeHTTP(w, r)
}

func downloadAndConvertImage(w http.ResponseWriter, r *http.Request, logger *customLogger) (err error) {
	//TODO: get asset info from immich and only download if JXL extension
	logger.SetErrPrefix("download and convert")
	logger.Printf("converting jxl: %s", r.URL)
	var req *http.Request
	var resp *http.Response
	var blob *os.File
	if req, err = http.NewRequest("GET", upstreamURL+r.URL.String(), nil); logger.Error(err, "new GET") {
		return
	}
	req.Header = r.Header
	if resp, err = getHTTPclient().Do(req); logger.Error(err, "getHTTPclient.Do") {
		return
	}
	if blob, err = os.CreateTemp("", "blob-*"); logger.Error(err, "blob create") {
		return
	}
	cleanupBlob := func() { blob.Close(); _ = os.Remove(blob.Name()) }
	defer cleanupBlob()
	if _, err = io.Copy(blob, resp.Body); logger.Error(err, "blob copy") {
		return
	}
	resp.Body.Close()
	if _, err = blob.Seek(0, io.SeekStart); logger.Error(err, "blob seek") {
		return
	}
	signature := make([]byte, 12)
	if _, err = blob.Read(signature); logger.Error(err, "blob read") {
		return
	}
	if bytes.Equal(signature, []byte{0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A}) {
		var output []byte
		var open *os.File
		if output, err = exec.Command("djxl", blob.Name(), blob.Name()+".jpg").CombinedOutput(); logger.Error(err, "djxl") {
			return
		}
		logger.Printf("conversion complete: %s", strings.ReplaceAll(string(output), "\n", " - "))
		cleanupBlob()
		if open, err = os.Open(blob.Name() + ".jpg"); logger.Error(err, "open jpg") {
			return
		}
		defer func() { open.Close(); _ = os.Remove(open.Name()) }()
		if _, err = io.Copy(w, open); logger.Error(err, "write resp") {
			return
		}
		return nil
	}
	return errors.New("not jxl")
}
