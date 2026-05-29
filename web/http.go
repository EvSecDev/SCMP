// Package for web interface backend server
package web

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"scmp/internal/logctx"
	"scmp/web/api"
	"scmp/web/datastore"
	"scmp/web/internal"
)

// Read in web static files at compile time
//
//go:embed static-files/*
var webFiles embed.FS

func StartListener(ctx context.Context, webConfigPath string) {
	// Recover from panic
	defer func() {
		if fatalError := recover(); fatalError != nil {
			fmt.Fprintf(os.Stderr, "Server panic: %v\n", fatalError)
			os.Exit(1)
		}
	}()

	ctx = logctx.AppendCtxTag(ctx, logctx.NSWeb)

	var webCfg internal.WebConfig
	err := webCfg.ExtractWebOptions(webConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error in controller web configuration: %v\n", err)
		os.Exit(1)
	}

	requestMultiplexer := http.NewServeMux()

	// Handle Health checks (authentication required)
	requestMultiplexer.HandleFunc("/health", func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		if clientRequest.Method == http.MethodGet {
			serverResponder.WriteHeader(http.StatusOK)
		}
	})

	// Put API catalog into global
	err = internal.WOAPIDef(api.SetupAPIEndpoints())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed global write for API catalog definition: %v\n", err)
		os.Exit(1)
	}

	// Handle API
	requestMultiplexer.HandleFunc("/api/", func(serverResponder http.ResponseWriter, clientRequest *http.Request) {
		// Call api with the base context (for global logging - user context should be extracted from client request)
		api.HandleAPI(ctx, serverResponder, clientRequest)
	})

	// Handle raw data up/down
	requestMultiplexer.HandleFunc("/data-store/", datastore.HandleBytes)

	// Handle static files - wrap with gzip handler
	staticFilesFS, err := fs.Sub(webFiles, "static-files")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to map embed fs to web fs: %v\n", err)
		os.Exit(1)
	}

	handleStatic := http.FileServer(http.FS(staticFilesFS))
	gzipHandler := compression(http.StripPrefix("/", handleStatic))
	requestMultiplexer.Handle("/", gzipHandler)

	// Declare middleware functions
	handlerWithMiddleware := chainMiddleware(
		requestMultiplexer,
		customErrorPage,
		authentication,
		validateReqHeaders,
		rateLimiter,
		addRespHeaders,
	)

	// Get socket for listening
	socket, err := validateListenSocket(internal.HTTPListenAddr, webCfg.HTTP.ListenPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse listen address: %v\n", err)
		os.Exit(1)
	}

	// Only allow HTTP2
	var allowedHTTPProtocols http.Protocols
	allowedHTTPProtocols.SetHTTP1(false)
	allowedHTTPProtocols.SetHTTP2(true)
	allowedHTTPProtocols.SetUnencryptedHTTP2(false)

	// Server configuration
	server := &http.Server{
		Addr:      socket,
		Handler:   handlerWithMiddleware,
		Protocols: &allowedHTTPProtocols,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13,
		},
		ReadTimeout:  internal.HTTPReadTimeout,
		WriteTimeout: internal.HTTPWriteTimeout,
		IdleTimeout:  internal.HTTPIdleTimeout,
		ErrorLog:     log.New(httpLogWriter{}, "", 0),
	}

	// Start the server with TLS
	logctx.LogStdInfo(ctx, "Server started on %s (https://%s:%d/)\n",
		socket,
		internal.HTTPListenAddr,
		webCfg.HTTP.ListenPort,
	)
	err = server.ListenAndServeTLS(webCfg.HTTP.TLSCertFile, webCfg.HTTP.TLSKeyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server listener: %v\n", err)
		os.Exit(1)
	}
}
