package tunnel

import "net/http"

var hopByHop = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func StripHop(h http.Header) http.Header {
	out := h.Clone()
	if out == nil {
		out = make(http.Header)
	}
	for _, extra := range out.Values("Connection") {
		out.Del(extra)
	}
	for _, name := range hopByHop {
		out.Del(name)
	}
	return out
}
