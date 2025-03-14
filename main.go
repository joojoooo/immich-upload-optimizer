package main

import (
	"bytes"
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

	"github.com/spf13/viper"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var remote *url.URL

var (
	maxConcurrentRequests = 10
	semaphore             = make(chan struct{}, maxConcurrentRequests)
)

var jobChannels = make(map[string]chan *http.Response)
var jobChannelsComplete = make(map[string]chan struct{})

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
	viper.SetDefault("listen", ":2283")
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
}

func validateInput() {
	if upstreamURL == "" {
		log.Fatal("the -upstream flag is required")
	}

	var err error
	remote, err = url.Parse(upstreamURL)
	if err != nil {
		log.Fatalf("invalid upstream URL: %v", err)
	}

	if configFile == "" {
		log.Fatal("the -tasks_file flag is required")
	}

	config, err = NewConfig(&configFile)
	if err != nil {
		log.Fatalf("error loading config file: %v", err)
	}
}

func main() {
	baseLogger := log.New(os.Stdout, "", log.Ldate|log.Ltime)

	proxy := httputil.NewSingleHostReverseProxy(remote)
	// Debug MITM proxy
	proxy.Transport = http.DefaultTransport
	proxyUrl, _ := url.Parse("http://localhost:8080")
	proxy.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyUrl)

	handler := func(w http.ResponseWriter, r *http.Request) {
		requestLogger := newCustomLogger(baseLogger, fmt.Sprintf("%s: ", strings.Split(r.RemoteAddr, ":")[0]))

		if r.URL.Path == "/_immich-upload-optimizer/wait" {
			continueJob(r, w, requestLogger)
			return
		}

		requestLogger.Printf("path: %s", r.URL.Path)
		switch r.Method {
		case "POST":
			// File upload path
			match, err := path.Match(filterPath, r.URL.Path)
			if err != nil {
				requestLogger.Printf("invalid filter_path: %s", r.URL)
				return
			}
			if match && strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				err = newJob(r, w, requestLogger)
				if err != nil {
					requestLogger.Printf("upload handler error: %v", err)
				}
				//TODO: save original file checksum
				return
			}
			// Full sync: replace extension and checksum
			match, _ = path.Match("/api/sync/full-sync", r.URL.Path)
			if match {
				client := &http.Client{}
				destination := *remote
				destination.Path = path.Join(destination.Path, r.URL.Path)
				req, _ := http.NewRequest("POST", destination.String(), nil)
				req.Header = r.Header
				req.Body = r.Body
				resp, _ := client.Do(req)
				respJson, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				newJson := bytes.ReplaceAll(respJson, []byte("jxl"), []byte("jpg"))
				newJson = bytes.ReplaceAll(newJson, []byte("Wew1zEcZSZRBVEQqVl7ouDDvbcE="), []byte("f5mnx6TkRuAcWA3WjKuOgKs2ceY="))
				w.Write(newJson)
				return
			}
			// Delta sync:
			// Albums:
		case "GET":
			// File download path
			match, _ := path.Match(filterPath+"/*/original", r.URL.Path)
			if match {
				//TODO: get file info and only download if JXL ext. Use tmp files and clean up
				requestLogger.Printf("downloading: %s", r.URL)
				client := &http.Client{}
				destination := *remote
				destination.Path = path.Join(destination.Path, r.URL.Path)
				req, _ := http.NewRequest("GET", destination.String(), nil)
				req.Header = r.Header
				resp, err := client.Do(req)
				if err != nil {
					requestLogger.Printf("download handler error: %v", err)
				}
				file, err := os.Create("./blob")
				if err != nil {
					requestLogger.Printf("failed to create file: %v", err)
				}
				defer file.Close()
				io.Copy(file, resp.Body)
				_, _ = file.Seek(0, io.SeekStart)
				signature := make([]byte, 12)
				_, _ = file.Read(signature)
				if bytes.Equal(signature, []byte{0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A}) {
					cmd := exec.Command("djxl", "./blob", "test.jpg")
					output, _ := cmd.CombinedOutput()
					requestLogger.Printf("download complete: %s %s", r.URL, output)
					open, _ := os.Open("./test.jpg")
					_, err = io.Copy(w, open)
					return
				}
			}
		}

		//requestLogger.Printf("proxy: %s", r.URL)

		r.Host = remote.Host
		proxy.ServeHTTP(w, r)
	}

	server := &http.Server{
		Addr:    listenAddr,
		Handler: http.HandlerFunc(handler),
	}

	log.Printf("Starting %s on %s...", printVersion(), listenAddr)
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir != "" {
		fmt.Println("tmp directory:", tmpDir)
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting immich-upload-optimizer: %v", err)
	}
}

func printVersion() string {
	return fmt.Sprintf("immich-upload-optimizer %s, commit %s, built at %s", version, commit, date)
}
