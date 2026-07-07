// Package myip は実行元のグローバルIPアドレスを取得する。
package myip

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Endpoint はグローバルIP取得先。テストで差し替えるためvarにしている。
var Endpoint = "https://checkip.amazonaws.com"

func Get(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get global ip from %s: %w", Endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get global ip: unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return "", fmt.Errorf("get global ip: invalid response %q", ip)
	}
	// 結果は /32 CIDR としてセキュリティグループのingressに使われるため、IPv4であることを要求する。
	if parsed.To4() == nil {
		return "", fmt.Errorf("get global ip: %q is not an IPv4 address", ip)
	}
	return ip, nil
}
