package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func HandleHttpInfo(w http.ResponseWriter, r *http.Request) {
	data := make(map[string]string)

	// Collect headers
	for k, v := range r.Header {
		switch strings.ToLower(k) {
		case "authorization", "proxy-authorization", "cookie", "set-cookie":
			data[k] = "[REDACTED]"
			continue
		}
		data[k] = strings.Join(v, ", ")
	}

	// Collect other request info
	data["Host"] = r.Host
	data["RemoteAddr"] = r.RemoteAddr
	data["Method"] = r.Method
	data["URL"] = r.URL.Path
	data["Protocol"] = r.Proto

	// Sort keys alphabetically
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Output
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%s: %s\n", k, data[k]))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(sb.String()))
}
