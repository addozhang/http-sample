package main

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const (
	IdentityHeader = "Identity"
	App            = "app"
	Version        = "version"
	Upstream       = "upstream"
	Port           = "port"
)

var upstream = os.Getenv(Upstream)
var appName = os.Getenv(App)
var version = os.Getenv(Version)
var port = os.Getenv(Port)

func main() {
	if port == "" {
		port = "8080"
	}

	// Handle SIGINT (CTRL+C) gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Set up OpenTelemetry.
	otelShutdown, err := setupOTelSDK(ctx, appName, version)

	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	// Start HTTP server.
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      newHTTPHandler(),
		WriteTimeout: 10 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for interruption.
	select {
	case err = <-srvErr:
		// Error when starting HTTP server.
		return
	case <-ctx.Done():
		// Wait for first CTRL+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}

	// When Shutdown is called, ListenAndServe immediately returns ErrServerClosed.
	err = srv.Shutdown(context.Background())
	return
}

func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// handleFunc is a replacement for mux.HandleFunc
	// which enriches the handler's HTTP instrumentation with the pattern as the http.route.
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// Configure the "http.route" for the HTTP instrumentation.
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}

	// Register handlers.
	handleFunc("/", handle)

	// Add HTTP instrumentation for the whole server.
	handler := otelhttp.NewHandler(mux, "/")
	return handler
}

func handle(w http.ResponseWriter, r *http.Request) {
	ip, hostname := getIPAndHostname()
	response := fmt.Sprintf("%s(version: %s, ip: %s, hostname: %s)", appName, version, ip, hostname)

	if upstream != "" {
		client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
		req, _ := http.NewRequestWithContext(r.Context(), "GET", upstream, nil)

		for name, value := range getTracingHeaders(r) {
			req.Header.Set(name, value)
		}

		upstreamResponse, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(w, "Error contacting upstream service: %v", err)
			return
		}
		defer upstreamResponse.Body.Close()
		body, err := ioutil.ReadAll(upstreamResponse.Body)
		if err != nil {
			fmt.Fprintf(w, "Error reading upstream response: %v", err)
			return
		}
		response += fmt.Sprintf(" -> %s", string(body))
	}

	setHeaders(w, r)
	fmt.Fprintf(w, response)
}

func getIPAndHostname() (string, string) {
	host, _ := os.Hostname()
	addrs, _ := net.LookupIP(host)
	var ip string
	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			ip = ipv4.String()
			break
		}
	}
	return ip, host
}

func setHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(IdentityHeader, os.Getenv(App))

	if r == nil {
		return
	}

	for _, header := range getTracingHeaderKeys() {
		if v := r.Header.Get(header); v != "" {
			w.Header().Set(header, v)
		}
	}
}

func getTracingHeaderKeys() []string {
	return []string{"X-Ot-Span-Context", "Traceparent", "X-Request-Id", "uber-trace-id", "x-b3-traceid", "x-b3-spanid", "x-b3-parentspanid"}
}

func getTracingHeaders(r *http.Request) map[string]string {
	var headers = map[string]string{}
	for _, key := range getTracingHeaderKeys() {
		if v := r.Header.Get(key); v != "" {
			headers[key] = v
		}
	}

	return headers
}
