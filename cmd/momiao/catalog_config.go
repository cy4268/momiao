package main

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var assetCDNOriginPattern = regexp.MustCompile(`^https://[a-zA-Z0-9](?:[a-zA-Z0-9.-]*[a-zA-Z0-9])?(?::[0-9]{1,5})?$`)

func catalogConfig(cfg *config, lookup func(string) (string, bool)) error {
	if value, ok := lookup("MOMIAO_ASSET_CDN_ORIGIN"); ok {
		u, err := url.Parse(value)
		if err != nil || !assetCDNOriginPattern.MatchString(value) {
			return errors.New("MOMIAO_ASSET_CDN_ORIGIN must be one explicit HTTPS origin without a path or wildcard")
		}
		if port := u.Port(); port != "" {
			n, err := strconv.Atoi(port)
			if err != nil || n < 1 || n > 65535 {
				return errors.New("invalid asset CDN port")
			}
		}
		cfg.AssetCDNOrigin = value
	}
	if value, ok := lookup("MOMIAO_API_BASE_URL"); ok {
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "/v1" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != value || strings.ContainsAny(value, "\r\n\t ") {
			return errors.New("API base URL must be an explicit HTTPS deployment URL ending in /v1")
		}
		cfg.APIBaseURL = value
	}
	return nil
}
