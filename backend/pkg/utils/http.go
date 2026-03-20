package utils

import (
	"net/http"
	"net/url"
	"time"

	"github.com/admin/message-router/internal/config"
	"golang.org/x/net/proxy"
)

func GetHTTPClient() *http.Client {
	proxyURL := config.AppConfig.Network.Proxy
	if proxyURL == "" {
		return &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Handle SOCKS5
	if u.Scheme == "socks5" {
		dialer, err := proxy.FromURL(u, proxy.Direct)
		if err != nil {
			return &http.Client{
				Timeout: 30 * time.Second,
			}
		}
		return &http.Client{
			Transport: &http.Transport{
				Dial: dialer.Dial,
			},
			Timeout: 30 * time.Second,
		}
	}

	// Handle HTTP/HTTPS proxy
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(u),
		},
		Timeout: 30 * time.Second,
	}
}
