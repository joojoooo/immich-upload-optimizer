package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"

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

var config *Config

func init() {
	viper.SetEnvPrefix("iuo")
	viper.AutomaticEnv()
	viper.BindEnv("upstream")
	viper.BindEnv("listen")
	viper.BindEnv("tasks_file")
	viper.BindEnv("filter_path")
	viper.BindEnv("filter_form_key")

	viper.SetDefault("upstream", "")
	viper.SetDefault("listen", ":2284")
	viper.SetDefault("tasks_file", "tasks.yaml")
	viper.SetDefault("filter_path", "/api/assets")
	viper.SetDefault("filter_form_key", "assetData")

	flag.BoolVar(&showVersion, "version", false, "Show the current version")
	flag.StringVar(&upstreamURL, "upstream", viper.GetString("upstream"), "Upstream URL. Example: http://immich-server:2283")
	flag.StringVar(&listenAddr, "listen", viper.GetString("listen"), "Listening address")
	flag.StringVar(&configFile, "tasks_file", viper.GetString("tasks_file"), "Path to the configuration file")
	flag.StringVar(&filterPath, "filter_path", viper.GetString("filter_path"), "Only convert files uploaded to specific path. Advanced, leave default for immich")
	flag.StringVar(&filterFormKey, "filter_form_key", viper.GetString("filter_form_key"), "Only convert files uploaded with specific form key. Advanced, leave default for immich")
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

const DevMITMproxy = true

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
	requestLogger := newCustomLogger(baseLogger, fmt.Sprintf("%s: ", strings.Split(r.RemoteAddr, ":")[0]))

	requestLogger.Printf("proxy path: %s", r.URL.Path)
	switch r.Method {
	case "POST":
		match, err := path.Match(filterPath, r.URL.Path)
		if err != nil {
			requestLogger.Printf("invalid filter_path: %s", r.URL)
			break
		}
		if match && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			err = newJob(r, w, requestLogger)
			if err != nil {
				requestLogger.Printf("upload error: %v", err)
			}
			return
		}
	case "GET":
	}

	r.Host = remote.Host
	proxy.ServeHTTP(w, r)
}
