package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
)

const (
	IdentityHeader = "Identity"
	App            = "app"
	Version        = "version"
	Upstream       = "upstream"
)

func main() {
	port := os.Getenv("port")
	if port == "" {
		port = "8080"
	}

	upstream := os.Getenv(Upstream)
	appName := os.Getenv(App)
	version := os.Getenv(Version)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ip, hostname := getIPAndHostname()
		response := fmt.Sprintf("%s(version: %s, ip: %s, hostname: %s)", appName, version, ip, hostname)

		if upstream != "" {

			client := &http.Client{}
			req, _ := http.NewRequest("GET", upstream, nil)

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
	})

	http.ListenAndServe(":"+port, nil)
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
	return []string{"X-Ot-Span-Context", "X-Request-Id", "uber-trace-id", "x-b3-traceid", "x-b3-spanid", "x-b3-parentspanid"}
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
