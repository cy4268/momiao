package relay

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Match the official file download bound; no disk cache or background worker.
const maxMediaBytes = 64 << 20

func init() {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		// Connect to the checked IP, not the hostname (also checks redirects).
		for _, ip := range addresses {
			ip = ip.Unmap()
			if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || netip.MustParsePrefix("100.64.0.0/10").Contains(ip) {
				return nil, errors.New("media address is not public")
			}
		}
		var dialErr error
		for _, ip := range addresses {
			conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = err
		}
		if dialErr == nil {
			dialErr = errors.New("media address not found")
		}
		return nil, dialErr
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	relayconvert.SetMediaResolver(relayconvert.MediaResolver{
		DecodeBase64FileData: decodeMedia,
		GetBase64Data: func(ctx context.Context, source types.FileSource, _ ...string) (string, string, error) {
			if !source.IsURL() {
				kind, data, err := decodeMedia(source.GetRawData())
				if inline, ok := source.(*types.Base64Source); ok && inline.MimeType != "" {
					kind = inline.MimeType
				}
				return data, kind, err
			}
			request, err := http.NewRequestWithContext(ctx, "GET", source.GetRawData(), nil)
			if err != nil {
				return "", "", err
			}
			response, err := client.Do(request)
			if err != nil {
				return "", "", err
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				return "", "", errors.New("media download failed")
			}
			data, err := io.ReadAll(io.LimitReader(response.Body, maxMediaBytes+1))
			if err != nil {
				return "", "", err
			}
			if len(data) > maxMediaBytes {
				return "", "", errors.New("media exceeds 64 MiB")
			}
			kind, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
			if kind == "" || kind == "application/octet-stream" {
				kind = http.DetectContentType(data)
			}
			return base64.StdEncoding.EncodeToString(data), kind, nil
		},
	})
}

// Same MIME/base64 contract as official service.DecodeBase64FileData; stdlib
// detection also works for PDFs/audio instead of assuming every file is an image.
func decodeMedia(raw string) (string, string, error) {
	kind := ""
	if strings.HasPrefix(raw, "data:") {
		prefix, value, ok := strings.Cut(raw, ",")
		if !ok || !strings.HasSuffix(prefix, ";base64") {
			return "", "", errors.New("invalid data URL")
		}
		kind = strings.TrimSuffix(strings.TrimPrefix(prefix, "data:"), ";base64")
		raw = value
	}
	if len(raw) > base64.StdEncoding.EncodedLen(maxMediaBytes) {
		return "", "", errors.New("media exceeds 64 MiB")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", "", err
	}
	if kind == "" {
		kind = http.DetectContentType(data)
	}
	return kind, raw, nil
}
