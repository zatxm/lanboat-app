package profile

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func (n Node) SudokuTables() []string {
	return strings.FieldsFunc(n.TextOption("custom-tables"), func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
}

func (n Node) NaiveHeaders() (map[string][]string, error) {
	headers := map[string][]string{}
	for _, line := range strings.Split(strings.ReplaceAll(n.TextOption("extra-headers"), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		value = strings.TrimSpace(value)
		if !ok || !httpguts.ValidHeaderFieldName(key) || !httpguts.ValidHeaderFieldValue(value) {
			return nil, fmt.Errorf("附加标头需每行填写 名称: 值")
		}
		key = http.CanonicalHeaderKey(key)
		if _, exists := headers[key]; exists {
			return nil, fmt.Errorf("附加标头名称不能重复")
		}
		headers[key] = []string{value}
	}
	return headers, nil
}

func (n Node) NaiveCertificatePEM() (string, error) {
	return CertificatePEM(n.TextOption("certificate"))
}

// CertificatePEM accepts a PEM certificate chain or a local file path.
func CertificatePEM(value string) (string, error) {
	content := strings.TrimSpace(value)
	if content == "" {
		return "", nil
	}
	if !strings.Contains(content, "-----BEGIN") {
		file, err := os.Open(content)
		if err != nil {
			return "", fmt.Errorf("无法读取证书链: %w", err)
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			return "", fmt.Errorf("证书链读取失败或超过 1 MiB")
		}
		content = strings.TrimSpace(string(data))
	}
	if content == "" {
		return "", fmt.Errorf("证书链不能为空")
	}
	for remaining := content; remaining != ""; {
		block, rest := pem.Decode([]byte(remaining))
		if !strings.HasPrefix(remaining, "-----BEGIN CERTIFICATE-----") || block == nil || block.Type != "CERTIFICATE" {
			return "", fmt.Errorf("证书链需为有效的 PEM 证书")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return "", fmt.Errorf("证书链包含无效的证书")
		}
		remaining = strings.TrimSpace(string(rest))
	}
	return content, nil
}

func validateExtraOptions(n Node) error {
	switch n.Kind() {
	case "sudoku":
		minimum, e1 := strconv.Atoi(n.EffectiveOption("padding-min", "5"))
		maximum, e2 := strconv.Atoi(n.EffectiveOption("padding-max", "15"))
		if e1 != nil || e2 != nil || minimum < 0 || maximum > 100 || maximum < minimum {
			return fmt.Errorf("填充比例需为 0–100，最大值不能小于最小值")
		}
		for _, table := range append(n.SudokuTables(), n.TextOption("custom-table")) {
			table = strings.ToLower(strings.TrimSpace(table))
			if table != "" && (len(table) != 8 || strings.Count(table, "x") != 2 || strings.Count(table, "p") != 2 || strings.Count(table, "v") != 4) {
				return fmt.Errorf("自定义编码表需包含 2 个 x、2 个 p 和 4 个 v")
			}
		}
		if n.TextOption("http-mask") == "true" && n.EffectiveOption("http-mask-mode", "auto") != "legacy" {
			if host := n.TextOption("http-mask-host"); host != "" && !ValidHost(host) {
				return fmt.Errorf("HTTP 伪装 Host 需为域名或 IP")
			}
		}
	case "naive":
		if _, err := n.NaiveHeaders(); err != nil {
			return err
		}
		if _, err := n.NaiveCertificatePEM(); err != nil {
			return err
		}
		if name := n.TextOption("sni"); name != "" && !ValidHost(name) {
			return fmt.Errorf("服务器名称需为域名或 IP")
		}
	case "tailscale":
		if value := n.TextOption("control-url"); value != "" {
			u, err := url.Parse(value)
			if err != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" {
				return fmt.Errorf("控制服务 URL 需为有效的 HTTP 或 HTTPS 地址")
			}
		}
	}
	return nil
}
