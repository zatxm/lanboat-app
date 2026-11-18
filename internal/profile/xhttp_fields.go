package profile

// xhttpFields is shared by every protocol using XHTTP transport or obfuscation.
func xhttpFields() []Field {
	text := func(key, hint string) Field { return placeholder(f("xhttp."+key, key, false), hint) }
	chooseX := func(key, def string, choices ...string) Field { return choose("xhttp."+key, key, def, choices...) }
	return []Field{
		boolean("xhttp.NoGRPCHeader", "NoGRPCHeader", "false"),
		boolean("xhttp.XPaddingObfsMode", "XPaddingObfsMode", "false"),
		text("XPaddingBytes", "100-1000"), text("XPaddingKey", "x_padding"), text("XPaddingHeader", "Referer"),
		chooseX("XPaddingPlacement", "queryInHeader", "queryInHeader", "cookie", "header", "query"),
		chooseX("XPaddingMethod", "repeat-x", "repeat-x", "tokenish"),
		text("UplinkHTTPMethod", "POST"),
		chooseX("SessionPlacement", "path", "path", "query", "cookie", "header"), text("SessionKey", ""),
		chooseX("SeqPlacement", "path", "path", "query", "cookie", "header"), text("SeqKey", ""),
		chooseX("UplinkDataPlacement", "body", "body", "cookie", "header"), text("UplinkDataKey", ""),
		text("UplinkChunkSize", "0"), text("ScMaxEachPostBytes", "1000000"),
		text("MaxConcurrency", "16-32"), text("MaxConnections", "0-0"), text("CMaxReuseTimes", "0-0"),
		text("HMaxReusableSecs", "1800-3000"), text("HKeepAlivePeriod", "45"),
		text("DownloadTargetHost", ""), text("DownloadTargetPort", "0"), text("DownloadServerName", ""), text("DownloadHTTPHost", ""),
	}
}
