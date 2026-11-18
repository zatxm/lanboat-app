package profile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type XHTTPExtra struct {
	Headers                map[string]string `json:"headers"`
	Padding                json.RawMessage   `json:"xPaddingBytes"`
	NoGRPCHeader           bool              `json:"noGRPCHeader"`
	PaddingMin, PaddingMax int               `json:"-"`
	PaddingObfs            bool              `json:"xPaddingObfsMode"`
	PaddingKey             string            `json:"xPaddingKey"`
	PaddingHeader          string            `json:"xPaddingHeader"`
	PaddingPlacement       string            `json:"xPaddingPlacement"`
	PaddingMethod          string            `json:"xPaddingMethod"`
	UplinkMethod           string            `json:"uplinkHTTPMethod"`
	SessionPlacement       string            `json:"sessionPlacement"`
	SessionKey             string            `json:"sessionKey"`
	SeqPlacement           string            `json:"seqPlacement"`
	SeqKey                 string            `json:"seqKey"`
	DataPlacement          string            `json:"uplinkDataPlacement"`
	DataKey                string            `json:"uplinkDataKey"`
	ChunkSize              string            `json:"uplinkChunkSize"`
	MaxPostBytes           string            `json:"scMaxEachPostBytes"`
	MaxConcurrency         string            `json:"maxConcurrency"`
	MaxConnections         string            `json:"maxConnections"`
	CMaxReuseTimes         string            `json:"cMaxReuseTimes"`
	HMaxReusableSecs       string            `json:"hMaxReusableSecs"`
	HKeepAlivePeriod       string            `json:"hKeepAlivePeriod"`
	DownloadTargetHost     string            `json:"downloadTargetHost"`
	DownloadTargetPort     string            `json:"downloadTargetPort"`
	DownloadServerName     string            `json:"downloadServerName"`
	DownloadHTTPHost       string            `json:"downloadHTTPHost"`
}

func (n Node) XHTTPExtraSettings() (XHTTPExtra, error) {
	extra := XHTTPExtra{PaddingMin: 100, PaddingMax: 1000}
	if text := n.TextOption("xhttp-extra"); text != "" {
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&extra); err != nil {
			return extra, fmt.Errorf("XHTTP Extra: %w", err)
		}
	}
	if v := n.Option("xhttp.NoGRPCHeader"); v != nil && fmt.Sprint(v) != "" {
		extra.NoGRPCHeader = fmt.Sprint(v) == "true"
	}
	if v := n.Option("xhttp.XPaddingObfsMode"); v != nil && fmt.Sprint(v) != "" {
		extra.PaddingObfs = fmt.Sprint(v) == "true"
	}
	set := func(key string, dest *string, def string) {
		if v := n.TextOption("xhttp." + key); v != "" {
			*dest = v
		}
		if *dest == "" {
			*dest = def
		}
	}
	set("XPaddingKey", &extra.PaddingKey, "x_padding")
	set("XPaddingHeader", &extra.PaddingHeader, "Referer")
	set("XPaddingPlacement", &extra.PaddingPlacement, "queryInHeader")
	set("XPaddingMethod", &extra.PaddingMethod, "repeat-x")
	set("UplinkHTTPMethod", &extra.UplinkMethod, "POST")
	extra.UplinkMethod = strings.ToUpper(extra.UplinkMethod)
	set("SessionPlacement", &extra.SessionPlacement, "path")
	set("SessionKey", &extra.SessionKey, "")
	set("SeqPlacement", &extra.SeqPlacement, "path")
	set("SeqKey", &extra.SeqKey, "")
	set("UplinkDataPlacement", &extra.DataPlacement, "body")
	set("UplinkDataKey", &extra.DataKey, "")
	set("UplinkChunkSize", &extra.ChunkSize, "")
	set("ScMaxEachPostBytes", &extra.MaxPostBytes, "")
	set("MaxConcurrency", &extra.MaxConcurrency, "")
	set("MaxConnections", &extra.MaxConnections, "")
	set("CMaxReuseTimes", &extra.CMaxReuseTimes, "")
	set("HMaxReusableSecs", &extra.HMaxReusableSecs, "")
	set("HKeepAlivePeriod", &extra.HKeepAlivePeriod, "")
	set("DownloadTargetHost", &extra.DownloadTargetHost, "")
	set("DownloadTargetPort", &extra.DownloadTargetPort, "")
	set("DownloadServerName", &extra.DownloadServerName, "")
	set("DownloadHTTPHost", &extra.DownloadHTTPHost, "")
	padding := n.TextOption("xhttp.XPaddingBytes")
	if padding == "" && len(extra.Padding) > 0 {
		padding = string(extra.Padding)
		if strings.HasPrefix(padding, "\"") {
			if err := json.Unmarshal(extra.Padding, &padding); err != nil {
				return extra, err
			}
		}
	}
	if padding != "" {
		var err error
		extra.PaddingMin, extra.PaddingMax, err = xhttpRange(padding, 1, 8192)
		if err != nil {
			return extra, fmt.Errorf("XPaddingBytes: %w", err)
		}
	}
	for _, item := range []struct {
		name, value string
		allowed     []string
	}{
		{"XPaddingPlacement", extra.PaddingPlacement, []string{"queryInHeader", "cookie", "header", "query"}},
		{"XPaddingMethod", extra.PaddingMethod, []string{"repeat-x", "tokenish"}},
		{"SessionPlacement", extra.SessionPlacement, []string{"path", "query", "cookie", "header"}},
		{"SeqPlacement", extra.SeqPlacement, []string{"path", "query", "cookie", "header"}},
		{"UplinkDataPlacement", extra.DataPlacement, []string{"body", "cookie", "header"}},
	} {
		found := false
		for _, v := range item.allowed {
			if item.value == v {
				found = true
			}
		}
		if !found {
			return extra, fmt.Errorf("%s 选项无效", item.name)
		}
	}
	for _, item := range []struct {
		name, value string
		max         int
	}{
		{"UplinkChunkSize", extra.ChunkSize, 1000000}, {"ScMaxEachPostBytes", extra.MaxPostBytes, 100000000},
		{"MaxConcurrency", extra.MaxConcurrency, 1000000}, {"MaxConnections", extra.MaxConnections, 1000000}, {"CMaxReuseTimes", extra.CMaxReuseTimes, 100000000}, {"HMaxReusableSecs", extra.HMaxReusableSecs, 100000000},
		{"HKeepAlivePeriod", extra.HKeepAlivePeriod, 100000000}, {"DownloadTargetPort", extra.DownloadTargetPort, 65535},
	} {
		if item.value != "" {
			if _, _, err := xhttpRange(item.value, 0, item.max); err != nil {
				return extra, fmt.Errorf("%s: %w", item.name, err)
			}
		}
	}
	for _, item := range []struct{ name, value string }{{"HKeepAlivePeriod", extra.HKeepAlivePeriod}, {"DownloadTargetPort", extra.DownloadTargetPort}} {
		if strings.Contains(item.value, "-") {
			return extra, fmt.Errorf("%s 必须是整数", item.name)
		}
	}
	if extra.MaxConcurrency != "" && extra.MaxConnections != "" {
		_, concurrency, _ := xhttpRange(extra.MaxConcurrency, 0, 1000000)
		_, connections, _ := xhttpRange(extra.MaxConnections, 0, 1000000)
		if concurrency > 0 && connections > 0 {
			return extra, fmt.Errorf("MaxConcurrency 和 MaxConnections 不能同时启用")
		}
	}
	for _, value := range []string{extra.PaddingKey, extra.PaddingHeader, extra.SessionKey, extra.SeqKey, extra.DataKey, extra.DownloadHTTPHost, extra.DownloadServerName} {
		if strings.ContainsAny(value, "\r\n") {
			return extra, fmt.Errorf("XHTTP 参数不能包含换行")
		}
	}
	if extra.DownloadTargetHost != "" && !ValidHost(extra.DownloadTargetHost) {
		return extra, fmt.Errorf("DownloadTargetHost 必须是域名或IP")
	}
	if extra.UplinkMethod == "" || strings.ContainsAny(extra.UplinkMethod, " \r\n\t") {
		return extra, fmt.Errorf("UplinkHTTPMethod 无效")
	}
	return extra, nil
}
func xhttpRange(value string, min, max int) (int, int, error) {
	first, last, ok := strings.Cut(value, "-")
	if !ok {
		last = first
	}
	lo, e1 := strconv.Atoi(first)
	hi, e2 := strconv.Atoi(last)
	if e1 != nil || e2 != nil || lo < min || hi < lo || hi > max {
		return 0, 0, fmt.Errorf("请输入 %d–%d 范围内的数值或范围", min, max)
	}
	return lo, hi, nil
}
