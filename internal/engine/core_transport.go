package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func transportOptions(n profile.Node, out map[string]any) {
	if n.Kind() == "ss" {
		mode := n.TextOption("obfs")
		if mode != "" && mode != "none" {
			switch mode {
			case "http", "tls", "ssl":
				if mode == "ssl" {
					mode = "tls"
				}
				if out["plugin"] == nil {
					out["plugin"] = "obfs-local"
					out["plugin_opts"] = "obfs=" + mode + ";obfs-host=" + sipEscape(n.TextOption("obfs-host")) + ";obfs-uri=" + sipEscape(n.EffectiveOption("obfs-path", "/"))
				} else {
					out["obfs"] = map[string]any{"type": mode, "host": n.TextOption("obfs-host"), "path": n.EffectiveOption("obfs-path", "/")}
				}
			default:
				kind := mode
				if kind == "h1" || kind == "h2" {
					kind = "http"
				}
				if kind == "websocket" || kind == "wss" {
					kind = "ws"
				}
				transport := map[string]any{"type": kind, "path": n.EffectiveOption("obfs-path", "/")}
				if host := n.TextOption("obfs-host"); host != "" {
					transport["headers"] = map[string]any{"Host": host}
					transport["host"] = host
				}
				out["transport"] = transport
				if mode == "wss" || mode == "h2" {
					out["tls"] = coreTLS(n)
					if mode == "h2" {
						out["tls"].(map[string]any)["alpn"] = []string{"h2"}
					}
				}
			}
		}
	}
	if n.TextOption("network") == "quic" && out["tls"] == nil {
		out["tls"] = coreTLS(n)
	}
	transport, ok := out["transport"].(map[string]any)
	if !ok {
		return
	}
	if n.TextOption("network") == "mkcp" {
		transport["type"] = "mkcp"
		opts := map[string]any{"header": map[string]any{"type": n.EffectiveOption("kcp.header", "none")}, "mtu": nodeInt(n, "kcp.mtu", 1350), "tti": nodeInt(n, "kcp.tti", 50), "congestion": n.TextOption("kcp.congestion") == "true"}
		for _, key := range []string{"uplink-capacity", "downlink-capacity"} {
			if n.TextOption("kcp."+key) != "" {
				opts[strings.ReplaceAll(key, "-", "_")] = nodeInt(n, "kcp."+key, 0)
			}
		}
		if seed := n.TextOption("kcp.seed"); seed != "" {
			opts["seed"] = seed
		}
		transport["kcp"] = opts
	}
	if transport["type"] != "xhttp" {
		return
	}
	transport["mode"] = n.EffectiveOption("xhttp-opts.mode", n.EffectiveOption("xhttp-mode", "auto"))
	transport["path"] = n.EffectiveOption("transport-path", n.EffectiveOption("obfs-path", "/"))
	if host := n.EffectiveOption("transport-host", n.TextOption("obfs-host")); host != "" {
		transport["host"] = host
	}
	extra, err := n.XHTTPExtraSettings()
	if err != nil {
		return
	}
	headers := map[string]any{}
	if original, ok := transport["headers"].(map[string]any); ok {
		for k, v := range original {
			if !strings.EqualFold(k, "Host") {
				headers[k] = v
			}
		}
	}
	for key, value := range extra.Headers {
		if strings.EqualFold(key, "Host") {
			if transport["host"] == nil {
				transport["host"] = value
			}
		} else {
			headers[key] = value
		}
	}
	delete(transport, "headers")
	if len(headers) > 0 {
		transport["headers"] = headers
	}
	values := map[string]any{"no_grpc_header": extra.NoGRPCHeader, "x_padding_obfs_mode": extra.PaddingObfs, "x_padding_bytes": fmt.Sprintf("%d-%d", extra.PaddingMin, extra.PaddingMax), "x_padding_key": extra.PaddingKey, "x_padding_header": extra.PaddingHeader, "x_padding_placement": extra.PaddingPlacement, "x_padding_method": extra.PaddingMethod, "uplink_http_method": extra.UplinkMethod, "session_placement": extra.SessionPlacement, "session_key": extra.SessionKey, "seq_placement": extra.SeqPlacement, "seq_key": extra.SeqKey, "uplink_data_placement": extra.DataPlacement, "uplink_data_key": extra.DataKey, "uplink_chunk_size": extra.ChunkSize, "sc_max_each_post_bytes": extra.MaxPostBytes, "download_target_host": extra.DownloadTargetHost, "download_server_name": extra.DownloadServerName, "download_http_host": extra.DownloadHTTPHost}
	for key, value := range values {
		if value != "" {
			transport[key] = value
		}
	}
	if extra.DownloadTargetPort != "" {
		transport["download_target_port"], _ = strconv.Atoi(extra.DownloadTargetPort)
	}
	mux := map[string]any{}
	for key, value := range map[string]string{"max_concurrency": extra.MaxConcurrency, "max_connections": extra.MaxConnections, "c_max_reuse_times": extra.CMaxReuseTimes, "h_max_reusable_secs": extra.HMaxReusableSecs} {
		if value != "" {
			mux[key] = value
		}
	}
	if extra.HKeepAlivePeriod != "" {
		mux["h_keep_alive_period"], _ = strconv.Atoi(extra.HKeepAlivePeriod)
	}
	if len(mux) > 0 {
		transport["xmux"] = mux
	}
}
func sipEscape(value string) string {
	return strings.NewReplacer("\\", "\\\\", ";", "\\;", "=", "\\=").Replace(value)
}
