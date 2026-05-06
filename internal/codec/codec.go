package codec

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

var (
	hasBrotli = true
	hasZstd   = true
)

func SupportedEncodings() string {
	codecs := []string{"gzip", "deflate"}
	if hasBrotli {
		codecs = append(codecs, "br")
	}
	if hasZstd {
		codecs = append(codecs, "zstd")
	}
	return strings.Join(codecs, ", ")
}

func Decode(body []byte, encoding string) []byte {
	if len(body) == 0 {
		return body
	}
	enc := strings.TrimSpace(strings.ToLower(encoding))
	if enc == "" || enc == "identity" {
		return body
	}
	if strings.Contains(enc, ",") {
		parts := strings.Split(enc, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			body = Decode(body, strings.TrimSpace(parts[i]))
		}
		return body
	}
	switch enc {
	case "gzip":
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return body
		}
		return out
	case "deflate":
		r, err := zlib.NewReader(bytes.NewReader(body))
		if err == nil {
			defer r.Close()
			out, err := io.ReadAll(r)
			if err == nil {
				return out
			}
		}
		return body
	case "br":
		if !hasBrotli {
			return body
		}
		r := brotli.NewReader(bytes.NewReader(body))
		out, err := io.ReadAll(r)
		if err != nil {
			return body
		}
		return out
	case "zstd":
		if !hasZstd {
			return body
		}
		r, err := zstd.NewReader(bytes.NewReader(body))
		if err != nil {
			return body
		}
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			return body
		}
		return out
	}
	return body
}
